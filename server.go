package main

import (
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const maxSize = 5242880

var blockNetworks = [][]byte{
	// IPv4 Link-local
	{169, 254},
	// IPv4 Private
	{10},
	{172, 16},
	{192, 168},
	// TODO: add IPv6
}

var httpClient = &http.Client{CheckRedirect: checkRedirect}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 4 {
		return errors.New("Stopped after 10 redirects")
	}
	if req.Host != "" && !hostAllowed(req.Host) {
		return errors.New("Invalid host")
	}
	return nil
}

func hostAllowed(host string) bool {
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
	networks:
		for _, net := range blockNetworks {
			for i, b := range net {
				// check each octet of the ip
				if ip[i] != b {
					// octet doesn't match, move on
					continue networks
				}
				// all prefix octets match, this network is blocked
				return false
			}
		}
	}

	return true
}

func proxyRequest(w http.ResponseWriter, r *http.Request) {
	if strings.Index(r.Header.Get("Via"), "assetproxy") != -1 {
		http.Error(w, "Requesting from self", http.StatusBadRequest)
	}
	if r.Method != "GET" {
		http.Error(w, "Only GET is allowed", http.StatusMethodNotAllowed)
	}

	destURL := r.URL.Query().Get("url")
	if destURL == "" {
		destURLBytes, err := hex.DecodeString(r.URL.Path[1:])
		if err != nil {
			http.Error(w, "Invalid URL encoding", http.StatusBadRequest)
			return
		}
		destURL = string(destURLBytes)
	}

	dest, err := url.Parse(destURL)
	if err != nil {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	if dest.Scheme != "http" && dest.Scheme != "https" {
		http.Error(w, "Invalid URL scheme, expected http", http.StatusBadRequest)
		return
	}
	if dest.Host == "" {
		http.Error(w, "Missing URL host", http.StatusBadRequest)
		return
	}
	if !hostAllowed(dest.Host) {
		http.Error(w, "Invalid host", http.StatusNotFound)
		return
	}

	acceptHeader := r.Header.Get("Accept")
	if acceptHeader == "" {
		acceptHeader = "image/*"
	}

	req, _ := http.NewRequest("GET", destURL, nil)
	req.Header.Set("User-Agent", r.Header.Get("User-Agent"))
	req.Header.Set("X-Content-Type-Options", "nosniff")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("Accept-Encoding", r.Header.Get("Accept-Encoding"))

	via := r.Header.Get("Via")
	if via != "" {
		via += ", "
	}
	via += "1.1 assetproxy"
	req.Header.Set("Via", via)

	ifModifiedSince := r.Header.Get("If-Modified-Since")
	if ifModifiedSince != "" {
		req.Header.Set("If-Modified-Since", ifModifiedSince)
	}

	ifNoneMatch := r.Header.Get("If-None-Match")
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNotModified {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	contentType := res.Header.Get("Content-Type")
	if len(contentType) < 5 || contentType[:5] != "image" {
		http.Error(w, "Received invalid Content-Type", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)

	etag := res.Header.Get("ETag")
	if etag != "" {
		w.Header().Set("ETag", etag)
	}

	contentEncoding := res.Header.Get("Content-Encoding")
	if contentEncoding != "" {
		w.Header().Set("Content-Encoding", contentEncoding)
	}

	contentLength := res.Header.Get("Content-Length")
	parsedContentLength, _ := strconv.Atoi(contentLength)
	if parsedContentLength > maxSize {
		http.Error(w, "Response is too large", http.StatusBadRequest)
		return
	}
	if contentLength != "" {
		w.Header().Set("Content-Length", contentLength)
	}

	cacheControl := res.Header.Get("Cache-Control")
	if cacheControl == "" {
		cacheControl = "public, max-age=3600"
	}
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	io.Copy(w, io.LimitReader(res.Body, maxSize))
}

func main() {
	http.HandleFunc("/", proxyRequest)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
