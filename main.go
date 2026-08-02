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
	"bjws":  "573ib1kp5nk92irinpumbo9krlb", // 北京卫视
	"btvxw": "53gpt1ephlp86eor6ahtkg5b2hf", // 北京新闻
	"btvty": "54hv0f3pq079d4oiil2k12dkvsc", // 北京体育休闲
	"btvwy": "54db6gi5vfj8r8q1e6r89imd64s", // 北京文艺
	"btvkj": "53bn9rlalq08lmb8nf8iadoph0b", // 北京纪实科教
	"btvys": "50mqo8t4n4e8gtarqr3orj9l93v", // 北京影视
	"btvcj": "50e335k9dq488lb7jo44olp71f5", // 北京财经
	"btvsh": "50j015rjrei9vmp3h8upblr41jf", // 北京生活
	"kaku":  "55skfjq618b9kcq9tfjr5qllb7r", // 卡酷少儿
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

// 🛡️ 自动检查与生成自签名 TLS 证书
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

// 🛡️ 安全解压函数：判断魔数，只有确定是 Gzip 才解压，避免非 Gzip 数据报错
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

// 输出单行 txt 订阅，返回纯净播放链接
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

// 输出 M3U 订阅，返回标准干净的链接（由后端统一处理代理）
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

// ---------------------------------------------------------
// 核心修改：拿到流地址后，代调并重写 M3U8 内的相对路径为完整 CDN 链接
// ---------------------------------------------------------
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

	// 1. 解压 API 返回的 JSONP 数据
	bodyBytes, _ = decompressGzip(bodyBytes)

	// 2. 正开提取 JSON
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

	// 3. 解密 base64 得到原生的 M3U8 链接
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

	// ------------------------------------------------------------------
	// 由 Go 服务带 Cookie 和 Referer 请求真实的 M3U8 文本
	// ------------------------------------------------------------------
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

	// ------------------------------------------------------------------
	// 关键改动点：解析出原 M3U8 的 Base URL，将所有相对切片地址修正为绝对路径
	// ------------------------------------------------------------------
	parsedStreamURL, err := url.Parse(streamURL)
	if err == nil {
		// 截取掉 URL 末尾的具体 m3u8 文件名，保留基础路径前缀
		baseURL := parsedStreamURL.Scheme + "://" + parsedStreamURL.Host + parsedStreamURL.Path
		if idx := strings.LastIndex(baseURL, "/"); idx != -1 {
			baseURL = baseURL[:idx+1]
		}

		lines := strings.Split(string(m3u8Body), "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			// 排除空行、注释行（#EXTINF...等）以及已经是完整 http(s) 的绝对路径
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
				if strings.HasPrefix(trimmed, "/") {
					// 针对以 / 开头的根路径相对地址
					lines[i] = parsedStreamURL.Scheme + "://" + parsedStreamURL.Host + trimmed
				} else {
					// 针对同级相对路径（如 btv_sn_...ts）
					lines[i] = baseURL + trimmed
				}
			}
		}
		m3u8Body = []byte(strings.Join(lines, "\n"))
	}

	// 将修复完相对路径后的 M3U8 文本返回给播放器
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write(m3u8Body)
}

func main() {
	mathrand.Seed(time.Now().UnixNano())

	// 1. 自动初始化局域网 HTTPS 证书
	if err := generateCertIfNotExist(); err != nil {
		fmt.Printf("Warning: Failed to generate TLS certificate: %v\n", err)
	}

	// 2. 默认调整为直接读取根目录下的 ./cookies.json
	cookieFile := "./cookies.json"
	if envFile := os.Getenv("COOKIES_FILE"); envFile != "" {
		cookieFile = envFile
	}

	if err := loadCookiesFromFile(cookieFile); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}

	if err := initSession(); err != nil {
		fmt.Printf("Warning: Failed to initialize session: %v\n", err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/playlist.txt" {
			handlePlaylistTxt(w, r)
		} else if r.URL.Path == "/playlist.m3u" || r.URL.Path == "/playlist" {
			handlePlaylistM3u(w, r)
		} else if r.URL.Path == "/" {
			if r.URL.Query().Get("id") != "" {
				handleResolver(w, r)
			} else {
				handlePlaylistM3u(w, r)
			}
		} else {
			http.NotFound(w, r)
		}
	})

	port := ":6600"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = ":" + envPort
	}

	// 优先启动 HTTPS 端口
	fmt.Printf("Server is running on HTTPS port %s...\n", strings.TrimPrefix(port, ":"))
	err := http.ListenAndServeTLS(port, certFile, keyFile, nil)
	if err != nil {
		fmt.Printf("TLS Server failed, fallback to HTTP: %v\n", err)
		if err := http.ListenAndServe(port, nil); err != nil {
			panic(err)
		}
	}
}