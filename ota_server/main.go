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

// Structured logging helper
func logInfo(msg string, fields ...string) {
	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z")
	logEntry := fmt.Sprintf("[%s] INFO: %s", timestamp, msg)
	if len(fields) > 0 {
		for i := 0; i < len(fields); i += 2 {
			if i+1 < len(fields) {
				logEntry += fmt.Sprintf(" %s=%s", fields[i], fields[i+1])
			}
		}
	}
	log.Println(logEntry)
}

func logError(msg string, err error, fields ...string) {
	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z")
	logEntry := fmt.Sprintf("[%s] ERROR: %s", timestamp, msg)
	if err != nil {
		logEntry += fmt.Sprintf(" error=%s", err.Error())
	}
	if len(fields) > 0 {
		for i := 0; i < len(fields); i += 2 {
			if i+1 < len(fields) {
				logEntry += fmt.Sprintf(" %s=%s", fields[i], fields[i+1])
			}
		}
	}
	log.Println(logEntry)
}

func logDebug(msg string, fields ...string) {
	if !debugMode {
		return
	}
	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z")
	logEntry := fmt.Sprintf("[%s] DEBUG: %s", timestamp, msg)
	if len(fields) > 0 {
		for i := 0; i < len(fields); i += 2 {
			if i+1 < len(fields) {
				logEntry += fmt.Sprintf(" %s=%s", fields[i], fields[i+1])
			}
		}
	}
	log.Println(logEntry)
}

// Enhanced logging middleware
func logRequest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		isHealthCheck := strings.Contains(r.UserAgent(), "kube-probe")

		// Log all requests in debug mode, only non-health-checks in normal mode
		if debugMode || !isHealthCheck {
			clientVersion := getClientVersion(r)
			logInfo("HTTP request started",
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
				"client_version", clientVersion,
			)
		}

		// Wrap response writer to capture status code
		wrapped := &responseWrapper{ResponseWriter: w, statusCode: 200}
		next(wrapped, r)

		duration := time.Since(start)

		if debugMode || !isHealthCheck {
			logInfo("HTTP request completed",
				"method", r.Method,
				"path", r.URL.Path,
				"status", fmt.Sprintf("%d", wrapped.statusCode),
				"duration_ms", fmt.Sprintf("%.2f", float64(duration.Nanoseconds())/1000000),
				"remote_addr", r.RemoteAddr,
			)
		}
	}
}

// Response wrapper to capture status code
type responseWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func main() {
	if err := os.MkdirAll(firmwareDir, 0755); err != nil {
		logError("Failed to create firmware directory", err, "path", firmwareDir)
		os.Exit(1)
	}

	http.HandleFunc("/upload", logRequest(uploadFirmware))
	http.HandleFunc("/firmware", logRequest(getFirmware))
	http.HandleFunc("/version", logRequest(getVersion))
	http.HandleFunc("/list", logRequest(listFirmware))
	http.HandleFunc("/delete", logRequest(deleteFirmware))

	logInfo("OTA Server starting",
		"port", "8080",
		"debug_mode", fmt.Sprintf("%v", debugMode),
		"firmware_dir", firmwareDir,
		"initial_version", versions.latest,
	)

	if err := http.ListenAndServe(":8080", nil); err != nil {
		logError("Server failed to start", err)
		os.Exit(1)
	}
}

// Upload new firmware version
func uploadFirmware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logError("Upload rejected - invalid method", nil, "method", r.Method, "remote_addr", r.RemoteAddr)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		logError("Upload failed - form parsing error", err, "remote_addr", r.RemoteAddr)
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("firmware")
	if err != nil {
		logError("Upload failed - file retrieval error", err, "remote_addr", r.RemoteAddr)
		http.Error(w, "Unable to retrieve file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	version := r.FormValue("version")
	if version == "" {
		logError("Upload failed - version not specified", nil, "remote_addr", r.RemoteAddr)
		http.Error(w, "Version not specified", http.StatusBadRequest)
		return
	}

	logInfo("Firmware upload started",
		"version", version,
		"filename", header.Filename,
		"size_bytes", fmt.Sprintf("%d", header.Size),
		"remote_addr", r.RemoteAddr,
	)

	// Update latest version and save file
	versions.Lock()
	oldVersion := versions.latest
	versions.latest = version
	versions.Unlock()

	firmwarePath := filepath.Join(firmwareDir, "firmware_"+version+".bin")
	dst, err := os.Create(firmwarePath)
	if err != nil {
		logError("Upload failed - file creation error", err,
			"version", version,
			"path", firmwarePath,
			"remote_addr", r.RemoteAddr,
		)
		http.Error(w, "Unable to create file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	bytesWritten, err := io.Copy(dst, file)
	if err != nil {
		logError("Upload failed - file write error", err,
			"version", version,
			"path", firmwarePath,
			"remote_addr", r.RemoteAddr,
		)
		http.Error(w, "Unable to save file", http.StatusInternalServerError)
		return
	}

	md5Hash := getMD5Hash(firmwarePath)
	logInfo("Firmware upload completed successfully",
		"version", version,
		"old_version", oldVersion,
		"filename", header.Filename,
		"size_bytes", fmt.Sprintf("%d", bytesWritten),
		"path", firmwarePath,
		"md5", md5Hash,
		"remote_addr", r.RemoteAddr,
	)

	fmt.Fprintf(w, "Firmware version %s uploaded successfully", version)
}

// Download firmware (latest or specific version)
func getFirmware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logError("Firmware download rejected - invalid method", nil, "method", r.Method, "remote_addr", r.RemoteAddr)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requestedVersion := r.URL.Query().Get("version")
	clientVersion := getClientVersion(r)

	versions.RLock()
	targetVersion := versions.latest
	versions.RUnlock()

	if requestedVersion != "" {
		targetVersion = requestedVersion
	}

	logDebug("Firmware download request",
		"client_version", clientVersion,
		"requested_version", requestedVersion,
		"target_version", targetVersion,
		"user_agent", r.UserAgent(),
		"remote_addr", r.RemoteAddr,
	)

	firmwarePath := filepath.Join(firmwareDir, "firmware_"+targetVersion+".bin")
	fileInfo, err := os.Stat(firmwarePath)
	if os.IsNotExist(err) {
		logError("Firmware download failed - file not found", nil,
			"target_version", targetVersion,
			"path", firmwarePath,
			"remote_addr", r.RemoteAddr,
		)
		http.Error(w, "Firmware not found", http.StatusNotFound)
		return
	}

	// Check if client already has latest version (ESP8266 OTA optimization)
	if requestedVersion == "" && clientVersion == targetVersion {
		logInfo("Firmware download - no update needed",
			"client_version", clientVersion,
			"latest_version", targetVersion,
			"remote_addr", r.RemoteAddr,
		)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Set firmware download headers
	md5Hash := getMD5Hash(firmwarePath)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=firmware.bin")
	w.Header().Set("x-MD5", md5Hash)

	logInfo("Firmware download started",
		"client_version", clientVersion,
		"target_version", targetVersion,
		"file_size", fmt.Sprintf("%d", fileInfo.Size()),
		"md5", md5Hash,
		"remote_addr", r.RemoteAddr,
	)

	http.ServeFile(w, r, firmwarePath)
}

// Get current firmware version (for health checks and ESP8266)
func getVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logError("Version request rejected - invalid method", nil, "method", r.Method, "remote_addr", r.RemoteAddr)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	versions.RLock()
	current := versions.latest
	versions.RUnlock()

	logDebug("Version request", "current_version", current, "remote_addr", r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": current})
}

// List all available firmware versions with metadata
func listFirmware(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logError("List request rejected - invalid method", nil, "method", r.Method, "remote_addr", r.RemoteAddr)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	files, err := os.ReadDir(firmwareDir)
	if err != nil {
		logError("List failed - directory read error", err, "firmware_dir", firmwareDir, "remote_addr", r.RemoteAddr)
		http.Error(w, "Unable to read firmware directory", http.StatusInternalServerError)
		return
	}

	versions.RLock()
	currentLatest := versions.latest
	versions.RUnlock()

	logInfo("Firmware list request",
		"current_version", currentLatest,
		"total_files", fmt.Sprintf("%d", len(files)),
		"remote_addr", r.RemoteAddr,
	)

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
	var totalSize int64

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".bin") {
			continue
		}

		filePath := filepath.Join(firmwareDir, file.Name())
		fileInfo, _ := file.Info()

		// Extract version from filename: firmware_X.X.X.bin
		version := strings.TrimPrefix(strings.TrimSuffix(file.Name(), ".bin"), "firmware_")
		totalSize += fileInfo.Size()

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

	logInfo("Firmware list prepared",
		"firmware_count", fmt.Sprintf("%d", len(firmwareList)),
		"total_size_bytes", fmt.Sprintf("%d", totalSize),
		"remote_addr", r.RemoteAddr,
	)

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
		logError("Delete request rejected - invalid method", nil, "method", r.Method, "remote_addr", r.RemoteAddr)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	version := r.URL.Query().Get("version")
	if version == "" {
		logError("Delete failed - version not specified", nil, "remote_addr", r.RemoteAddr)
		http.Error(w, "Version not specified", http.StatusBadRequest)
		return
	}

	firmwarePath := filepath.Join(firmwareDir, "firmware_"+version+".bin")

	// Check if file exists before deletion
	if _, err := os.Stat(firmwarePath); os.IsNotExist(err) {
		logError("Delete failed - firmware not found", nil,
			"version", version,
			"path", firmwarePath,
			"remote_addr", r.RemoteAddr,
		)
		http.Error(w, "Firmware not found", http.StatusNotFound)
		return
	}

	if err := os.Remove(firmwarePath); err != nil {
		logError("Delete failed - file removal error", err,
			"version", version,
			"path", firmwarePath,
			"remote_addr", r.RemoteAddr,
		)
		http.Error(w, "Failed to delete firmware", http.StatusInternalServerError)
		return
	}

	logInfo("Firmware deleted successfully",
		"version", version,
		"path", firmwarePath,
		"remote_addr", r.RemoteAddr,
	)

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
		logError("MD5 calculation failed - file open error", err, "path", filePath)
		return ""
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		logError("MD5 calculation failed - hash calculation error", err, "path", filePath)
		return ""
	}

	result := fmt.Sprintf("%x", hash.Sum(nil))
	logDebug("MD5 calculated", "path", filePath, "md5", result)
	return result
}
