	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/playlist.txt" {
			handlePlaylistTxt(w, r)
		} else if r.URL.Path == "/playlist.m3u" {
			handlePlaylistM3u(w, r)
		} else if r.URL.Path == "/" && r.RawQuery == "" {
			// / 返回 M3U 播放列表
			handlePlaylistM3u(w, r)
		} else if r.URL.Path == "/" && r.URL.Query().Get("id") != "" {
			// /?id=xxx 返回直播流
			handleResolver(w, r)
		} else if strings.HasSuffix(r.URL.Path, ".ts") {
			handleTsProxy(w, r)
		} else {
			http.NotFound(w, r)
		}
	})
