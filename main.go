package main

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/big"
	mathrand "math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	dataDir  = "./data"
	certFile = "./data/server.crt"
	keyFile  = "./data/server.key"
)

var channelIds = map[string]string{
	"bjws":  "573ib1kp5nk92irinpumbo9krlb",
	"btvxw": "53gpt1ephlp86eor6ahtkg5b2hf",
	"btvty": "54hv0f3pq079d4oiil2k12dkvsc",
	"btvwy": "54db6gi5vfj8r8q1e6r89imd64s",
	"btvkj": "53bn9rlalq08lmb8nf8iadoph0b",
	"btvys": "50mqo8t4n4e8gtarqr3orj9l93v",
	"btvcj": "50e335k9dq488lb7jo44olp71f5",
	"btvsh": "50j015rjrei9vmp3h8upblr41jf",
	"kaku":  "55skfjq618b9kcq9tfjr5qllb7r",
}

type CookieConfig struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	HttpOnly bool   `json:"httpOnly"`
}

type CookiesFile struct {
	Cookies []CookieConfig `json:"cookies"`
}

type StreamItem struct {
	StreamURL string `json:"stream_url"`
}

type APIResponse struct {
	Errno int `json:"errno"`
	Data  struct {
		VideoStream []StreamItem `json:"video_stream"`
	} `json:"data"`
}

var globalClient *http.Client

func init() {
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 15 * time.Second,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		DisableCompression:  false,
	}
	globalClient = &http.Client{
		Jar:       jar,
		Timeout:   30 * time.Second,
		Transport: transport,
	}
}

func generateCertIfNotExist() error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %v", err)
	}
	_, errCert := os.Stat(certFile)
	_, errKey := os.Stat(keyFile)
	if errCert == nil && errKey == nil {
		log.Println("[TLS] 证书与私钥校验通过，跳过生成步骤。")
		return nil
	}
	log.Println("[TLS] 未找到证书，正在自动生成 10 年期的局域网自签名 TLS 证书...")
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("生成私钥失败: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"IPTV Local Server"},
			CommonName:   "IPTV TLS Server",
		},
		NotBefore:             time.Now().Add(-10 * time.Minute),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					template.IPAddresses = append(template.IPAddresses, ipnet.IP)
				}
			}
		}
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("生成证书失败: %v", err)
	}
	certOut, err := os.Create(certFile)
	if err != nil {
		return fmt.Errorf("保存 cert 失败: %v", err)
	}
	defer certOut.Close()
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return fmt.Errorf("保存 key 失败: %v", err)
	}
	defer keyOut.Close()
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	log.Println("[TLS] 自签名证书成功生成并保存在目录:", dataDir)
	return nil
}

func buildBtimeURL(id string, typeID int, from string) string {
	secret := "TtJSg@2g*$K4PjUH"
	timestamp := time.Now().Unix()
	rawSig := fmt.Sprintf("%s%d%d%s", id, typeID, timestamp, secret)
	hash := md5.Sum([]byte(rawSig))
	sign := hex.EncodeToString(hash[:])[:8]
	cbRand1 := mathrand.Int63n(900000000000000000) + 100000000000000000
	cbRand2 := mathrand.Intn(90) + 10
	callback := fmt.Sprintf("jQuery%d_%d%d", cbRand1, timestamp, cbRand2)
	_rand := fmt.Sprintf("%d%d", timestamp, mathrand.Intn(90)+10)
	params := url.Values{}
	params.Set("from", from)
	params.Set("callback", callback)
	params.Set("id", id)
	params.Set("type_id", strconv.Itoa(typeID))
	params.Set("timestamp", strconv.FormatInt(timestamp, 10))
	params.Set("sign", sign)
	params.Set("_", _rand)
	return "https://pc.api.btime.com/video/play?" + params.Encode()
}

func loadCookiesFromFile(filepath string) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read cookies file: %v", err)
	}
	var cf CookiesFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return fmt.Errorf("failed to parse cookies JSON: %v", err)
	}
	u, _ := url.Parse("https://www.btime.com/")
	var cookies []*http.Cookie
	for _, c := range cf.Cookies {
		cookies = append(cookies, &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HttpOnly: c.HttpOnly,
		})
	}
	globalClient.Jar.SetCookies(u, cookies)
	fmt.Printf("Loaded %d cookies from %s\n", len(cookies), filepath)
	return nil
}

func areCookiesValid() bool {
	const cookieFile = "./cookies.json"
	data, err := os.ReadFile(cookieFile)
	if err != nil {
		return false
	}
	var cf CookiesFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return false
	}
	var usid, lid string
	for _, c := range cf.Cookies {
		if c.Name == "usid" {
			usid = c.Value
		}
		if c.Name == "__lid" {
			lid = c.Value
		}
	}
	return usid != "" && lid != ""
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "localhost"
}

// getDisplayIP 优先使用环境变量 HOST_IP，否则自动获取本地 IP
func getDisplayIP() string {
	if hostIP := os.Getenv("HOST_IP"); hostIP != "" {
		return hostIP
	}
	return getLocalIP()
}

func initSession() error {
	req, err := http.NewRequest("GET", "https://www.btime.com/", nil)
	if err != nil {
		return err
	}
	setCommonHeaders(req)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	resp, err := globalClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	fmt.Println("Session initialized")
	return nil
}

func setCommonHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
}

func decompressGzip(data []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return data, nil
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func decodeStreamURL(s string) string {
	if strings.Contains(s, "//") {
		return s
	}
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	rev := string(runes)
	b1, err := base64.StdEncoding.DecodeString(rev)
	if err != nil {
		return s
	}
	b2, err := base64.StdEncoding.DecodeString(string(b1))
	if err != nil {
		return s
	}
	return string(b2)
}

func processStreamURL(s string) string {
	if strings.Contains(s, "//") {
		return s
	}
	return decodeStreamURL(s)
}

var jsonpRegex = regexp.MustCompile(`\(([\{\[].*[\}\]])\)\s*;?$`)

func loadPlaylist(filename string, r *http.Request) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	content := string(data)
	content = strings.ReplaceAll(content, "${replace}", r.Host)
	return content, nil
}

func handlePlaylistTxt(w http.ResponseWriter, r *http.Request) {
	content, err := loadPlaylist("./channels.txt", r)
	if err != nil {
		http.Error(w, "Failed to load playlist", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, content)
}

func handlePlaylistM3u(w http.ResponseWriter, r *http.Request) {
	content, err := loadPlaylist("./playlist.txt", r)
	if err != nil {
		http.Error(w, "Failed to load playlist", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, content)
}

func handleResolver(w http.ResponseWriter, r *http.Request) {
	idKey := r.URL.Query().Get("id")
	if strings.Contains(idKey, "|") {
		idKey = strings.Split(idKey, "|")[0]
	}
	gid, exists := channelIds[idKey]
	if !exists {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	apiURL := buildBtimeURL(gid, 151, "pc")
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		http.Error(w, "API fetch failed", http.StatusBadGateway)
		return
	}
	setCommonHeaders(req)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", "https://www.btime.com")
	req.Header.Set("Referer", "https://www.btime.com/")
	resp, err := globalClient.Do(req)
	if err != nil {
		http.Error(w, "API fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "API fetch failed", http.StatusBadGateway)
		return
	}
	bodyBytes, _ = decompressGzip(bodyBytes)
	matches := jsonpRegex.FindSubmatch(bodyBytes)
	if len(matches) < 2 {
		http.Error(w, "Bad JSONP", http.StatusBadGateway)
		return
	}
	var payload APIResponse
	if err := json.Unmarshal(matches[1], &payload); err != nil {
		http.Error(w, "API error", http.StatusBadGateway)
		return
	}
	if payload.Errno != 0 || len(payload.Data.VideoStream) == 0 {
		http.Error(w, "No streams", http.StatusNotFound)
		return
	}
	var streamURL string
	for _, st := range payload.Data.VideoStream {
		if st.StreamURL == "" {
			continue
		}
		streamURL = processStreamURL(st.StreamURL)
		if streamURL != "" {
			break
		}
	}
	if streamURL == "" {
		http.Error(w, "No URL", http.StatusNotFound)
		return
	}
	m3u8Req, err := http.NewRequest("GET", streamURL, nil)
	if err != nil {
		http.Error(w, "Failed to create stream request", http.StatusInternalServerError)
		return
	}
	setCommonHeaders(m3u8Req)
	m3u8Req.Header.Set("Referer", "https://www.btime.com/")
	m3u8Resp, err := globalClient.Do(m3u8Req)
	if err != nil || m3u8Resp.StatusCode != http.StatusOK {
		http.Error(w, "Failed to fetch stream from BTime", http.StatusBadGateway)
		return
	}
	defer m3u8Resp.Body.Close()
	m3u8Body, err := io.ReadAll(m3u8Resp.Body)
	if err != nil {
		http.Error(w, "Read M3U8 content failed", http.StatusInternalServerError)
		return
	}
	parsedStreamURL, err := url.Parse(streamURL)
	if err == nil {
		baseURL := parsedStreamURL.Scheme + "://" + parsedStreamURL.Host + parsedStreamURL.Path
		if idx := strings.LastIndex(baseURL, "/"); idx != -1 {
			baseURL = baseURL[:idx+1]
		}
		lines := strings.Split(string(m3u8Body), "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
				if strings.HasPrefix(trimmed, "/") {
					lines[i] = parsedStreamURL.Scheme + "://" + parsedStreamURL.Host + trimmed
				} else {
					lines[i] = baseURL + trimmed
				}
			}
		}
		m3u8Body = []byte(strings.Join(lines, "\n"))
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write(m3u8Body)
}

func handleCookies(w http.ResponseWriter, r *http.Request) {
	const cookieFile = "./cookies.json"
	getCurrentValues := func() (usid, lid string, err error) {
		data, err := os.ReadFile(cookieFile)
		if err != nil {
			return "", "", err
		}
		var cf CookiesFile
		if err := json.Unmarshal(data, &cf); err != nil {
			return "", "", err
		}
		for _, c := range cf.Cookies {
			if c.Name == "usid" {
				usid = c.Value
			}
			if c.Name == "__lid" {
				lid = c.Value
			}
		}
		return usid, lid, nil
	}
	updateCookies := func(newUsid, newLid string) error {
		data, err := os.ReadFile(cookieFile)
		if err != nil {
			return err
		}
		var cf CookiesFile
		if err := json.Unmarshal(data, &cf); err != nil {
			return err
		}
		for i, c := range cf.Cookies {
			if c.Name == "usid" {
				cf.Cookies[i].Value = newUsid
			}
			if c.Name == "__lid" {
				cf.Cookies[i].Value = newLid
			}
		}
		newData, err := json.MarshalIndent(cf, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(cookieFile, newData, 0644)
	}
	if r.Method == http.MethodPost {
		usid := r.FormValue("usid")
		lid := r.FormValue("lid")
		if err := updateCookies(usid, lid); err != nil {
			http.Error(w, "保存失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := loadCookiesFromFile(cookieFile); err != nil {
			log.Printf("重新加载 Cookies 失败: %v", err)
		}
		http.Redirect(w, r, "/cookies?success=true", http.StatusSeeOther)
		return
	}
	usid, lid, err := getCurrentValues()
	if err != nil {
		http.Error(w, "读取 Cookies 失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data := struct {
		Usid    string
		Lid     string
		Success bool
	}{
		Usid:    usid,
		Lid:     lid,
		Success: r.URL.Query().Get("success") == "true",
	}
	t, err := template.ParseFiles("cookies.html")
	if err != nil {
		http.Error(w, "加载模板失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		log.Printf("模板渲染错误: %v", err)
	}
}

func handleHelp(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("./help/help.html")
	if err != nil {
		http.Error(w, "加载帮助页面失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t.Execute(w, nil)
}

func main() {
	mathrand.Seed(time.Now().UnixNano())

	if err := generateCertIfNotExist(); err != nil {
		fmt.Printf("Warning: Failed to generate TLS certificate: %v\n", err)
	}

	cookieFile := "./cookies.json"
	if envFile := os.Getenv("COOKIES_FILE"); envFile != "" {
		cookieFile = envFile
	}
	if err := loadCookiesFromFile(cookieFile); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}

	// 检查 Cookie 是否有效
	if !areCookiesValid() {
		displayIP := getDisplayIP()
		port := "6600"
		if envPort := os.Getenv("PORT"); envPort != "" {
			port = envPort
		}
		fmt.Printf("\n⚠️  Cookie 未配置或无效！\n")
		// 提示链接改为 http:// 协议
		fmt.Printf("   请访问 http://%s:%s/cookies 填入 usid 和 __lid 的值。\n", displayIP, port)
		if os.Getenv("HOST_IP") == "" {
			fmt.Printf("   （若从其他设备访问，请将 %s 替换为宿主机的实际 IP）\n", displayIP)
		}
		fmt.Println()
	}

	if err := initSession(); err != nil {
		fmt.Printf("Warning: Failed to initialize session: %v\n", err)
	}

	http.HandleFunc("/cookies", handleCookies)
	http.HandleFunc("/help", handleHelp)
	http.Handle("/help/static/", http.StripPrefix("/help/static/", http.FileServer(http.Dir("./help/images"))))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlist.txt":
			handlePlaylistTxt(w, r)
		case "/playlist.m3u", "/playlist":
			handlePlaylistM3u(w, r)
		case "/":
			if r.URL.Query().Get("id") != "" {
				handleResolver(w, r)
			} else {
				handlePlaylistM3u(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	})

	port := ":6600"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = ":" + envPort
	}

	// 启动纯 HTTP 服务，使得播放软件能够无障碍通过 http:// 访问
	log.Printf("Server is running on HTTP port %s...\n", strings.TrimPrefix(port, ":"))
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("HTTP Server failed: %v", err)
	}
}
