# ESP8266 OTA Server

A lightweight Over-The-Air (OTA) firmware update server for ESP8266 devices.

## Features

- 📦 **Upload** firmware versions with automatic versioning
- 📥 **Download** latest or specific firmware versions
- 🔍 **List** all available firmware with metadata
- 🗑️ **Delete** specific firmware versions
- ✅ **Version checking** - ESP8266 devices get 304 Not Modified if already up-to-date
- 🔐 **MD5 verification** for firmware integrity
- 🏥 **Health checks** for Kubernetes deployment
- 🔇 **Silent mode** - health checks don't spam logs (set `DEBUG=true` for verbose logging)

## Quick Start

```bash
# Run locally
go run main.go

# With debug logging
DEBUG=true go run main.go

# Docker build
docker build -t ota-server .

# Docker run
docker run -p 8080:8080 -v ./firmware:/app/firmware ota-server
```

## API Endpoints

### Upload Firmware
```bash
POST /upload
curl -X POST -F "firmware=@firmware.bin" -F "version=1.2.3" http://localhost:8080/upload
```

### Download Firmware
```bash
# Latest version
GET /firmware
curl -o firmware.bin http://localhost:8080/firmware

# Specific version
GET /firmware?version=1.2.3
curl -o firmware.bin http://localhost:8080/firmware?version=1.2.3
```

### Get Current Version
```bash
GET /version
curl http://localhost:8080/version
# {"version": "1.2.3"}
```

### List All Firmware
```bash
GET /list
curl http://localhost:8080/list
```

### Delete Firmware Version
```bash
DELETE /delete?version=1.2.3
curl -X DELETE http://localhost:8080/delete?version=1.2.3
```

## ESP8266 Integration

Add this to your ESP8266 sketch:

```cpp
#include <ESP8266httpUpdate.h>

void checkForUpdate() {
  WiFiClient client;
  t_httpUpdate_return ret = ESPhttpUpdate.update(client, 
    "http://your-server:8080/firmware", 
    FIRMWARE_VERSION);
    
  switch (ret) {
    case HTTP_UPDATE_OK:
      ESP.restart();
      break;
    case HTTP_UPDATE_NO_UPDATES:
      Serial.println("Already up-to-date");
      break;
    case HTTP_UPDATE_FAILED:
      Serial.println("Update failed");
      break;
  }
}
```

## Environment Variables

- `DEBUG=true` - Enable verbose logging including health checks
- `PORT=8080` - Server port (default: 8080)
- `FIRMWARE_DIR=./firmware` - Firmware storage directory

## Kubernetes Deployment

The server includes health check endpoints suitable for Kubernetes:

```yaml
livenessProbe:
  httpGet:
    path: /version
    port: 8080
readinessProbe:
  httpGet:
    path: /version
    port: 8080
```

Health check requests from `kube-probe` are automatically silenced unless `DEBUG=true`.

## File Structure

```
firmware/
├── firmware_1.2.1.bin
├── firmware_1.2.2.bin
└── firmware_1.2.3.bin  # latest
```

## Response Examples

### List Response
```json
{
  "current_version": "1.2.3",
  "firmware_count": 3,
  "firmware_list": [
    {
      "version": "1.2.3",
      "filename": "firmware_1.2.3.bin",
      "size": 245760,
      "modified": "2025-08-29 12:34:56",
      "md5": "abc123...",
      "is_latest": true,
      "download_url": "/firmware?version=1.2.3"
    }
  ]
}
```
