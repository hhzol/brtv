package main

import (
	"compress/gzip"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
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

var channelIds = map[string]string{
	"bjws":  "573ib1kp5nk92irinpumbo9krlb",
	"btvwy": "54db6gi5vfj8r8q1e6r89imd64s",
	"btvkj": "53bn9rlalq08lmb8nf8iadoph0b",
	"btvys": "50mqo8t4n4e8gtarqr3orj9l93v",
	"btvcj": "50e335k9dq488lb7jo44olp71f5",
	"btvsh": "50j015rjrei9vmp3h8upblr41jf",
	"btvqn": "53grctge7jb8aeamggnot6fve1o",
	"btvxw": "53gpt1ephlp86eor6ahtkg5b2hf",
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
var currentM3U8URL string
var currentStreamHost string

func init() {
	jar, _ := cookiejar.New(nil)

	transport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 15 * time.Second,
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

func buildBtimeURL(id string, typeID int, from string) string {
	secret := "TtJSg@2g*$K4PjUH"
	timestamp := time.Now().Unix()

	rawSig := fmt.Sprintf("%s%d%d%s", id, typeID, timestamp, secret)
	hash := md5.Sum([]byte(rawSig))
	sign := hex.EncodeToString(hash[:])[:8]

	cbRand1 := rand.Int63n(900000000000000000) + 100000000000000000
	cbRand2 := rand.Intn(90) + 10
	callback := fmt.Sprintf("jQuery%d_%d%d", cbRand1, timestamp, cbRand2)

	_rand := fmt.Sprintf("%d%d", timestamp, rand.Intn(90)+10)

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
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
}

func decompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(strings.NewReader(string(data)))
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

func rewriteM3U8(m3u8Content string, proxyHost string) string {
	lines := strings.Split(m3u8Content, "\n")
	var result []string

	for _, line := range lines {
		if strings.HasSuffix(strings.TrimSpace(line), ".ts") || strings.Contains(line, ".ts?") {
			if strings.HasPrefix(line, "http") {
				parts := strings.Split(line, "/")
				filename := parts[len(parts)-1]
				newURL := fmt.Sprintf("http://%s/%s", proxyHost, filename)
				fmt.Printf("Rewriting TS URL: %s -> %s\n", line, newURL)
				result = append(result, newURL)
			} else {
				result = append(result, line)
			}
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

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

	// 设置为纯文本输出，不包含 Content-Disposition 触发下载
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, content)
	fmt.Printf("Playlist (TXT) returned (host: %s)\n", r.Host)
}

func handlePlaylistM3u(w http.ResponseWriter, r *http.Request) {
	content, err := loadPlaylist("./playlist.txt", r)
	if err != nil {
		http.Error(w, "Failed to load playlist", http.StatusInternalServerError)
		return
	}

	// 设置为纯文本输出，以便浏览器能够直接展示内容而不是下载文件
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, content)
	fmt.Printf("Playlist (M3U) returned (host: %s)\n", r.Host)
}

func handleResolver(w http.ResponseWriter, r *http.Request) {
	idKey := r.URL.Query().Get("id")
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

	fmt.Printf("Request URL: %s\n", apiURL)

	setCommonHeaders(req)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", "https://www.btime.com")
	req.Header.Set("Referer", "https://www.btime.com/")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")

	resp, err := globalClient.Do(req)
	if err != nil {
		fmt.Printf("Request error: %v\n", err)
		http.Error(w, "API fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Read body error: %v\n", err)
		http.Error(w, "API fetch failed", http.StatusBadGateway)
		return
	}

	bodyBytes, _ = decompressGzip(bodyBytes)
	fmt.Printf("API Response: %s\n", string(bodyBytes))

	matches := jsonpRegex.FindSubmatch(bodyBytes)
	if len(matches) < 2 {
		http.Error(w, "Bad JSONP", http.StatusBadGateway)
		return
	}

	var payload APIResponse
	if err := json.Unmarshal(matches[1], &payload); err != nil {
		fmt.Printf("JSON parse error: %v\n", err)
		http.Error(w, "API error", http.StatusBadGateway)
		return
	}

	if payload.Errno != 0 {
		fmt.Printf("API errno: %d\n", payload.Errno)
		http.Error(w, "API error", http.StatusBadGateway)
		return
	}

	if len(payload.Data.VideoStream) == 0 {
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

	fmt.Printf("Decoded stream URL: %s\n", streamURL)

	proxyReq, err := http.NewRequest("GET", streamURL, nil)
	if err != nil {
		http.Error(w, "Proxy failed", http.StatusBadGateway)
		return
	}

	setCommonHeaders(proxyReq)
	proxyReq.Header.Set("Referer", "https://www.btime.com/")
	proxyReq.Header.Set("Origin", "https://www.btime.com")

	proxyResp, err := globalClient.Do(proxyReq)
	if err != nil {
		fmt.Printf("Stream fetch error: %v\n", err)
		http.Error(w, "Stream fetch failed", http.StatusBadGateway)
		return
	}
	defer proxyResp.Body.Close()

	m3u8Data, err := io.ReadAll(proxyResp.Body)
	if err != nil {
		http.Error(w, "Failed to read M3U8", http.StatusBadGateway)
		return
	}

	m3u8Content := string(m3u8Data)
	fmt.Printf("M3U8 content length: %d\n", len(m3u8Content))

	u, _ := url.Parse(streamURL)
	currentStreamHost = u.Host
	currentM3U8URL = streamURL

	clientHost := r.Host
	if r.Header.Get("X-Forwarded-For") != "" {
		clientHost = r.Header.Get("X-Forwarded-For")
	}

	m3u8Content = rewriteM3U8(m3u8Content, clientHost)

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, m3u8Content)

	fmt.Printf("M3U8 rewritten and returned\n")
}

func handleTsProxy(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Path[1:]
	queryString := r.URL.RawQuery

	realURL := fmt.Sprintf("https://%s/live/%s", currentStreamHost, filename)
	if queryString != "" {
		realURL = fmt.Sprintf("%s?%s", realURL, queryString)
	}

	fmt.Printf("Proxying TS: %s\n", realURL)

	proxyReq, err := http.NewRequest("GET", realURL, nil)
	if err != nil {
		http.Error(w, "Proxy failed", http.StatusBadGateway)
		return
	}

	setCommonHeaders(proxyReq)
	proxyReq.Header.Set("Referer", "https://www.btime.com/")
	proxyReq.Header.Set("Origin", "https://www.btime.com")
	proxyReq.Header.Set("Range", r.Header.Get("Range"))

	proxyResp, err := globalClient.Do(proxyReq)
	if err != nil {
		fmt.Printf("TS fetch error: %v\n", err)
		http.Error(w, "Failed to fetch TS", http.StatusBadGateway)
		return
	}
	defer proxyResp.Body.Close()

	for key, values := range proxyResp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(proxyResp.StatusCode)

	io.Copy(w, proxyResp.Body)
	fmt.Printf("TS proxied successfully (status: %d)\n", proxyResp.StatusCode)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	cookieFile := "/app/config/cookies.json"
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
		} else if strings.HasSuffix(r.URL.Path, ".ts") {
			handleTsProxy(w, r)
		} else {
			http.NotFound(w, r)
		}
	})

	port := ":6600"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = ":" + envPort
	}

	fmt.Printf("Server is running on port %s...\n", strings.TrimPrefix(port, ":"))
	if err := http.ListenAndServe(port, nil); err != nil {
		panic(err)
	}
}