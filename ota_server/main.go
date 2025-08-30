package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	firmwareDir = "./firmware"
	debugMode   = os.Getenv("DEBUG") == "true"
	versions    = struct {
		sync.RWMutex
		latest string
	}{latest: "1.1.0"}
)

// Simple logging middleware - only logs non-health-check requests or when DEBUG=true
func logRequest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isHealthCheck := strings.Contains(r.UserAgent(), "kube-probe") || r.URL.Path == "/version"

		if debugMode || !isHealthCheck {
			start := time.Now()
			next(w, r)
			log.Printf("%s %s - %s (%v)", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
		} else {
			next(w, r)
		}
	}
}

func main() {
	if err := os.MkdirAll(firmwareDir, 0755); err != nil {
		log.Fatalf("Failed to create firmware directory: %v", err)
	}

	http.HandleFunc("/upload", logRequest(uploadFirmware))
	http.HandleFunc("/firmware", logRequest(getFirmware))
	http.HandleFunc("/version", logRequest(getVersion))
	http.HandleFunc("/list", logRequest(listFirmware))
	http.HandleFunc("/delete", logRequest(deleteFirmware))

	log.Printf("🚀 OTA Server started on :8080 (DEBUG=%v)", debugMode)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// Upload new firmware version
func uploadFirmware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("firmware")
	if err != nil {
		http.Error(w, "Unable to retrieve file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	version := r.FormValue("version")
	if version == "" {
		http.Error(w, "Version not specified", http.StatusBadRequest)
		return
	}

	// Update latest version and save file
	versions.Lock()
	versions.latest = version
	versions.Unlock()

	firmwarePath := filepath.Join(firmwareDir, "firmware_"+version+".bin")
	dst, err := os.Create(firmwarePath)
	if err != nil {
		http.Error(w, "Unable to create file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "Unable to save file", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Firmware v%s uploaded (%s, %d bytes)", version, header.Filename, header.Size)
	fmt.Fprintf(w, "Firmware version %s uploaded successfully", version)
}

// Download firmware (latest or specific version)
func getFirmware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requestedVersion := r.URL.Query().Get("version")

	versions.RLock()
	targetVersion := versions.latest
	versions.RUnlock()

	if requestedVersion != "" {
		targetVersion = requestedVersion
	}

	firmwarePath := filepath.Join(firmwareDir, "firmware_"+targetVersion+".bin")
	fileInfo, err := os.Stat(firmwarePath)
	if os.IsNotExist(err) {
		http.Error(w, "Firmware not found", http.StatusNotFound)
		return
	}

	// Check if client already has latest version (ESP8266 OTA optimization)
	clientVersion := getClientVersion(r)
	if requestedVersion == "" && clientVersion == targetVersion {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Set firmware download headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=firmware.bin")
	w.Header().Set("x-MD5", getMD5Hash(firmwarePath))

	if debugMode {
		log.Printf("📥 Serving firmware v%s (%d bytes)", targetVersion, fileInfo.Size())
	}

	http.ServeFile(w, r, firmwarePath)
}

// Get current firmware version (for health checks and ESP8266)
func getVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	versions.RLock()
	current := versions.latest
	versions.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": current})
}

// List all available firmware versions with metadata
func listFirmware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	files, err := os.ReadDir(firmwareDir)
	if err != nil {
		http.Error(w, "Unable to read firmware directory", http.StatusInternalServerError)
		return
	}

	versions.RLock()
	currentLatest := versions.latest
	versions.RUnlock()

	type FirmwareInfo struct {
		Version     string `json:"version"`
		Filename    string `json:"filename"`
		Size        int64  `json:"size"`
		Modified    string `json:"modified"`
		MD5         string `json:"md5"`
		IsLatest    bool   `json:"is_latest"`
		DownloadURL string `json:"download_url"`
	}

	var firmwareList []FirmwareInfo
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".bin") {
			continue
		}

		filePath := filepath.Join(firmwareDir, file.Name())
		fileInfo, _ := file.Info()

		// Extract version from filename: firmware_X.X.X.bin
		version := strings.TrimPrefix(strings.TrimSuffix(file.Name(), ".bin"), "firmware_")

		firmwareList = append(firmwareList, FirmwareInfo{
			Version:     version,
			Filename:    file.Name(),
			Size:        fileInfo.Size(),
			Modified:    fileInfo.ModTime().Format("2006-01-02 15:04:05"),
			MD5:         getMD5Hash(filePath),
			IsLatest:    version == currentLatest,
			DownloadURL: "/firmware?version=" + version,
		})
	}

	response := map[string]interface{}{
		"current_version": currentLatest,
		"firmware_count":  len(firmwareList),
		"firmware_list":   firmwareList,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Delete specific firmware version
func deleteFirmware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	version := r.URL.Query().Get("version")
	if version == "" {
		http.Error(w, "Version not specified", http.StatusBadRequest)
		return
	}

	firmwarePath := filepath.Join(firmwareDir, "firmware_"+version+".bin")
	if err := os.Remove(firmwarePath); err != nil {
		http.Error(w, "Firmware not found", http.StatusNotFound)
		return
	}

	log.Printf("🗑️ Firmware v%s deleted", version)
	fmt.Fprintf(w, "Firmware version %s deleted successfully", version)
}

// Extract client version from ESP8266 request headers
func getClientVersion(r *http.Request) string {
	if headerVersion := r.Header.Get("x-esp8266-version"); headerVersion != "" {
		return headerVersion
	}

	userAgent := r.UserAgent()
	if parts := strings.Split(userAgent, "/"); len(parts) > 1 {
		return parts[1]
	}

	return ""
}

// Calculate MD5 hash for firmware integrity verification
func getMD5Hash(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	hash := md5.New()
	io.Copy(hash, file)
	return fmt.Sprintf("%x", hash.Sum(nil))
}
