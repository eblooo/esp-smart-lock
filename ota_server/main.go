package main

import (
	"crypto/md5"
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
	versions    = struct {
		sync.RWMutex
		latest string
	}{latest: "1.1.0"} // Default version - matches ESP8266 current version
)

// Logging middleware
func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Log incoming request
		log.Printf("→ [%s] %s %s - Remote: %s, User-Agent: %s",
			start.Format("15:04:05"), r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())

		// Create a custom ResponseWriter to capture status code
		wrapped := &responseWrapper{ResponseWriter: w, statusCode: 200}

		// Call the actual handler
		next(wrapped, r)

		// Log response
		duration := time.Since(start)
		log.Printf("← [%s] %s %s - Status: %d, Duration: %v",
			time.Now().Format("15:04:05"), r.Method, r.URL.Path, wrapped.statusCode, duration)
	}
}

// responseWrapper captures the status code
type responseWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func main() {
	// Create the firmware directory if it doesn't exist
	if err := os.MkdirAll(firmwareDir, 0755); err != nil {
		log.Fatalf("Failed to create firmware directory: %v", err)
	}

	// Define HTTP routes with logging middleware
	http.HandleFunc("/upload", loggingMiddleware(uploadFirmwareHandler))
	http.HandleFunc("/firmware", loggingMiddleware(getFirmwareHandler))
	http.HandleFunc("/version", loggingMiddleware(getVersionHandler))
	http.HandleFunc("/list", loggingMiddleware(listFirmwareHandler))

	// Start the server
	log.Printf("🚀 OTA Server started at http://localhost:8080")
	log.Printf("📁 Firmware directory: %s", firmwareDir)
	log.Printf("📡 Endpoints:")
	log.Printf("   POST /upload   - Upload firmware (requires 'firmware' file and 'version' field)")
	log.Printf("   GET  /firmware - Download latest firmware")
	log.Printf("   GET  /version  - Get current firmware version")
	log.Printf("   GET  /list     - List all firmware versions with metadata")
	log.Printf("===============================================")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func uploadFirmwareHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("❌ Upload failed: Method not allowed (%s)", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil { // Limit upload size to 10MB
		log.Printf("❌ Upload failed: Unable to parse form - %v", err)
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	// Retrieve the firmware file from the form data
	file, header, err := r.FormFile("firmware")
	if err != nil {
		log.Printf("❌ Upload failed: Unable to retrieve file - %v", err)
		http.Error(w, "Unable to retrieve the file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Extract version from form data
	version := r.FormValue("version")
	if version == "" {
		log.Printf("❌ Upload failed: Version not specified")
		http.Error(w, "Version not specified", http.StatusBadRequest)
		return
	}

	log.Printf("📦 Uploading firmware: version=%s, filename=%s, size=%d bytes",
		version, header.Filename, header.Size)

	// Update the latest version
	versions.Lock()
	oldVersion := versions.latest
	versions.latest = version
	versions.Unlock()

	// Save the uploaded file
	firmwarePath := filepath.Join(firmwareDir, "firmware_"+version+".bin")
	dst, err := os.Create(firmwarePath)
	if err != nil {
		log.Printf("❌ Upload failed: Unable to create file %s - %v", firmwarePath, err)
		http.Error(w, "Unable to create the file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		log.Printf("❌ Upload failed: Unable to save file - %v", err)
		http.Error(w, "Unable to save the file", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Firmware uploaded successfully: %s → %s (was: %s)",
		header.Filename, firmwarePath, oldVersion)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Firmware version %s uploaded successfully", version)
}

func getFirmwareHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("❌ Firmware download failed: Method not allowed (%s)", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if specific version is requested
	requestedVersion := r.URL.Query().Get("version")

	versions.RLock()
	latestVersion := versions.latest
	versions.RUnlock()

	// Use requested version or latest
	targetVersion := latestVersion
	if requestedVersion != "" {
		targetVersion = requestedVersion
		log.Printf("📋 Specific version requested: %s", requestedVersion)
	}

	firmwarePath := filepath.Join(firmwareDir, "firmware_"+targetVersion+".bin")

	// Check if the firmware file exists
	fileInfo, err := os.Stat(firmwarePath)
	if os.IsNotExist(err) {
		log.Printf("❌ Firmware download failed: File not found - %s", firmwarePath)
		if requestedVersion != "" {
			http.Error(w, fmt.Sprintf("Firmware version %s not found", requestedVersion), http.StatusNotFound)
		} else {
			http.Error(w, "Firmware file not found", http.StatusNotFound)
		}
		return
	}

	// Extract version from ESP8266httpUpdate User-Agent header
	// Format: "ESP8266-http-Update" or "ESP8266-http-Update/1.1.0"
	userAgent := r.UserAgent()
	clientVersion := ""

	// Parse version from User-Agent if present
	if len(userAgent) > 0 && userAgent != "ESP8266-http-Update" {
		parts := strings.Split(userAgent, "/")
		if len(parts) > 1 {
			clientVersion = parts[1]
		}
	}

	// Also check x-esp8266-version header (alternative method)
	if headerVersion := r.Header.Get("x-esp8266-version"); headerVersion != "" {
		clientVersion = headerVersion
	}

	log.Printf("📋 Firmware request: client_version=%s, target_version=%s, user_agent=%s",
		clientVersion, targetVersion, userAgent)

	// Only do version checking if no specific version was requested and it's an ESP8266 request
	if requestedVersion == "" && clientVersion != "" && clientVersion == latestVersion {
		log.Printf("✅ Client already has latest version (%s), returning 304 Not Modified", latestVersion)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Set headers for proper firmware download
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=firmware.bin")
	w.Header().Set("x-MD5", getMD5Hash(firmwarePath)) // ESP8266 can verify integrity

	if requestedVersion != "" {
		log.Printf("📥 Serving specific firmware version: %s, path=%s, size=%d bytes",
			targetVersion, firmwarePath, fileInfo.Size())
	} else if clientVersion != "" {
		log.Printf("📥 Serving firmware update: %s → %s, path=%s, size=%d bytes",
			clientVersion, targetVersion, firmwarePath, fileInfo.Size())
	} else {
		log.Printf("📥 Serving firmware: version=%s, path=%s, size=%d bytes",
			targetVersion, firmwarePath, fileInfo.Size())
	}

	// Serve the firmware file
	http.ServeFile(w, r, firmwarePath)
}

func getVersionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("❌ Version request failed: Method not allowed (%s)", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	versions.RLock()
	latestVersion := versions.latest
	versions.RUnlock()

	log.Printf("📋 Version requested: current=%s", latestVersion)

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"version": "%s"}`, latestVersion)
}

func listFirmwareHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("❌ List request failed: Method not allowed (%s)", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read firmware directory
	files, err := os.ReadDir(firmwareDir)
	if err != nil {
		log.Printf("❌ List failed: Unable to read firmware directory - %v", err)
		http.Error(w, "Unable to read firmware directory", http.StatusInternalServerError)
		return
	}

	versions.RLock()
	currentLatest := versions.latest
	versions.RUnlock()

	log.Printf("📋 Firmware list requested - found %d files", len(files))

	// Build firmware list with metadata
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
		fileInfo, err := file.Info()
		if err != nil {
			log.Printf("⚠️ Could not get info for file %s: %v", file.Name(), err)
			continue
		}

		// Extract version from filename (format: firmware_X.X.X.bin)
		version := "unknown"
		if strings.HasPrefix(file.Name(), "firmware_") && strings.HasSuffix(file.Name(), ".bin") {
			version = strings.TrimPrefix(strings.TrimSuffix(file.Name(), ".bin"), "firmware_")
		}

		firmwareInfo := FirmwareInfo{
			Version:     version,
			Filename:    file.Name(),
			Size:        fileInfo.Size(),
			Modified:    fileInfo.ModTime().Format("2006-01-02 15:04:05"),
			MD5:         getMD5Hash(filePath),
			IsLatest:    version == currentLatest,
			DownloadURL: fmt.Sprintf("/firmware?version=%s", version),
		}

		firmwareList = append(firmwareList, firmwareInfo)
	}

	// Sort by version (latest first)
	// Simple sort to put latest version first
	for i := 0; i < len(firmwareList); i++ {
		if firmwareList[i].IsLatest && i != 0 {
			// Move latest to front
			latest := firmwareList[i]
			copy(firmwareList[1:i+1], firmwareList[0:i])
			firmwareList[0] = latest
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")

	// Build JSON response manually for better formatting
	response := fmt.Sprintf(`{
  "current_version": "%s",
  "firmware_count": %d,
  "firmware_list": [`, currentLatest, len(firmwareList))

	for i, fw := range firmwareList {
		if i > 0 {
			response += ","
		}
		response += fmt.Sprintf(`
    {
      "version": "%s",
      "filename": "%s",
      "size": %d,
      "modified": "%s",
      "md5": "%s",
      "is_latest": %t,
      "download_url": "%s"
    }`, fw.Version, fw.Filename, fw.Size, fw.Modified, fw.MD5, fw.IsLatest, fw.DownloadURL)
	}

	response += `
  ]
}`

	fmt.Fprint(w, response)
	log.Printf("✅ Firmware list served: %d versions found", len(firmwareList))
}

// getMD5Hash calculates MD5 hash of a file for integrity verification
func getMD5Hash(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Failed to open file for MD5: %v", err)
		return ""
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		log.Printf("Failed to calculate MD5: %v", err)
		return ""
	}

	return fmt.Sprintf("%x", hash.Sum(nil))
}
