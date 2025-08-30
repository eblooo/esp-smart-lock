# ESP Smart Lock - ESP8266 Smart Lock Controller with OTA Updates

A complete smart lock control system powered by ESP8266 microcontroller with Over-The-Air (OTA) firmware updates, automated CI/CD pipeline, Kubernetes deployment, and UDP remote control interface.

## 🌟 Features

- **Smart Lock Control**: Remote control of electrical locks via UDP commands
- **Automated OTA Updates**: ESP8266 devices automatically check for and install firmware updates
- **CI/CD Pipeline**: GitHub Actions automatically builds and deploys firmware on code changes
- **Kubernetes Deployment**: Scalable OTA server running on Kubernetes
- **UDP Command Interface**: Remote lock control via network commands
- **OLED Display**: Real-time status display showing lock state and system info
- **Web Interface**: HTTP endpoints for device status and manual updates
- **Security**: SSL/TLS enabled with automatic certificate management

## 🏗️ Architecture

### Components

1. **ESP8266 Smart Lock Controller** (`sketch/sketch.ino`)
   - WiFi-enabled microcontroller controlling electrical lock
   - Listens for UDP unlock commands on port 2333
   - OLED display shows lock status and system information
   - Automatically checks for OTA updates every hour
   - Web server for status monitoring and manual control

2. **OTA Server** (`ota_server/main.go`)
   - Go-based HTTP server for firmware management
   - Handles firmware uploads and downloads
   - Version management and integrity checking
   - RESTful API for device management

3. **Kubernetes Deployment** (`k8s-ota-server/`)
   - Production-ready deployment manifests
   - Ingress with SSL termination
   - Health checks and monitoring

4. **Client Script** (`client.py`)
   - Python UDP client for sending unlock commands to ESP8266
   - Used for remote lock control over network

5. **CI/CD Pipeline** (`.github/workflows/build.yml`)
   - Automated firmware compilation
   - Version management with build numbers
   - Automatic deployment to OTA server

## 🚀 Quick Start

### Prerequisites

- ESP8266 development board (NodeMCU v2 recommended)
- Electrical lock or relay module for lock control
- Arduino IDE or Arduino CLI
- Python 3.x
- Kubernetes cluster (for production deployment)
- Go 1.22+ (for local OTA server development)

### 1. Hardware Setup

**Required Components:**
- ESP8266 NodeMCU v2
- SSD1306 OLED display (128x64)
- Electrical lock or relay module
- 5V/12V power supply (depending on lock requirements)
- Breadboard and jumper wires
- Transistor or relay driver circuit (if needed)

**Wiring:**
```
ESP8266 NodeMCU v2 -> OLED Display
D6 (GPIO12)        -> SCL
D5 (GPIO14)        -> SDA
3V3                -> VCC
GND                -> GND

ESP8266 NodeMCU v2 -> Lock Control
D2 (GPIO4)         -> Relay/Lock Control Pin
                      (through appropriate driver circuit)
VIN/5V             -> Lock Power Supply (+)
GND                -> Lock Power Supply (-)

Note: Use appropriate relay or transistor driver circuit
between ESP8266 and electrical lock for proper isolation
and current handling.
```

### 2. Configure WiFi Credentials

Edit `sketch/sketch.ino` and update WiFi credentials:

```cpp
const char* ssid = "your_wifi_network";     // Your WiFi SSID
const char* password = "your_wifi_password"; // Your WiFi password
```

### 3. Deploy OTA Server

#### Option A: Kubernetes Deployment (Production)

1. Apply Kubernetes manifests:
```bash
kubectl apply -f k8s-ota-server/ota-esp8622/
```

2. Update ingress host in `04.ingress.yaml` to your domain:
```yaml
- host: your-domain.com  # Change this to your domain
```

#### Option B: Local Development

1. Build and run the OTA server:
```bash
cd ota_server
go run main.go
```

The server will start on `http://localhost:8080`

### 4. Upload Initial Firmware

#### Option A: Using GitHub Actions (Recommended)

1. Fork the ESP Smart Lock repository
2. Update the OTA server URL in `.github/workflows/build.yml`:
```yaml
SERVER_URL: https://your-domain.com/upload  # Change to your server
```
3. Push changes to trigger automatic build and deployment

#### Option B: Manual Upload

1. Compile the sketch using Arduino IDE or CLI
2. Upload firmware manually:
```bash
curl -X POST \
  -F "firmware=@build/sketch.ino.bin" \
  -F "version=1.0.0" \
  http://your-server.com/upload
```

### 5. Flash Initial Firmware

1. Open `sketch/sketch.ino` in Arduino IDE
2. Select board: "NodeMCU 1.0 (ESP-12E Module)"
3. Upload the sketch to your ESP8266

### 6. Test Lock Control

Update the IP address in `client.py` to match your ESP8266's IP:

```python
server_address = ('192.168.1.100', 2333)  # ESP8266's IP address
```

Run the client to unlock the door:
```bash
python client.py
```

The ESP8266 will receive the unlock command and activate the electrical lock.

## 📖 Usage

### Lock Control

Send UDP commands to unlock the electrical lock:

```python
# Send unlock command to open the lock
python client.py  # Sends "unlock" command

# Custom commands can be added by modifying client.py
```

**Available Commands:**
- `unlock`: Opens the electrical lock and displays "T - ON" message
- `f_pressed`: Alternative trigger command for lock control

### Remote Lock Operation

The system operates as follows:
1. Send UDP packet with "unlock" command to ESP8266
2. ESP8266 receives command and activates lock control pin (GPIO4)
3. Electrical lock opens for configured duration
4. OLED display shows lock status and operation count
5. Lock automatically re-engages after timeout

### OTA Server API

The OTA server provides several HTTP endpoints:

**Get Current Version:**
```bash
curl https://your-server.com/version
```

**List All Firmware Versions:**
```bash
curl https://your-server.com/list
```

**Upload New Firmware:**
```bash
curl -X POST \
  -F "firmware=@firmware.bin" \
  -F "version=1.2.3" \
  https://your-server.com/upload
```

**Download Firmware:**
```bash
curl -O https://your-server.com/firmware
curl -O https://your-server.com/firmware?version=1.2.3
```

### Device Web Interface

Each ESP8266 smart lock controller runs a web server accessible at its IP address:

**Lock Status:**
```bash
curl http://192.168.1.100/
```

**Manual Lock Control:**
```bash
curl -X POST http://192.168.1.100/update
```

## 🔧 Development

### Modifying Firmware

1. Edit `sketch/sketch.ino`
2. Test locally using Arduino IDE
3. Commit and push changes
4. GitHub Actions will automatically build and deploy

### Adding New Lock Commands

1. Add command handling in `sketch/sketch.ino`:
```cpp
else if (strcmp(packetBuffer, "lock_status") == 0) {
  // Handle lock status request
  printDisplay("Lock Status", "Secure");
}
```

2. Update `client.py` to send the new command:
```python
message = b'lock_status'
```

### Local OTA Server Development

1. Make changes to `ota_server/main.go`
2. Test locally:
```bash
cd ota_server
go run main.go
```
3. Build Docker image:
```bash
docker build -t your-registry/ota-server:latest .
```

## 🔒 Security Considerations

- **WiFi Security**: Use WPA3/WPA2 with strong passwords
- **OTA Updates**: HTTPS endpoints with valid SSL certificates
- **Network Isolation**: Place smart lock devices on separate VLAN
- **Access Control**: Implement IP whitelisting for lock commands
- **Firmware Validation**: Server validates firmware integrity using MD5 checksums
- **Physical Security**: Secure the ESP8266 controller in tamper-resistant enclosure
- **Lock Failsafe**: Ensure lock fails to secure state on power loss

## 🐛 Troubleshooting

### ESP8266 Not Connecting to WiFi

1. Check WiFi credentials in `sketch.ino`
2. Ensure 2.4GHz WiFi is available (ESP8266 doesn't support 5GHz)
3. Check serial monitor for connection status

### OTA Updates Failing

1. Verify OTA server is accessible from ESP8266
2. Check server logs for upload/download errors
3. Ensure firmware size doesn't exceed ESP8266 memory limits
4. Verify version formatting is correct

### UDP Commands Not Working

1. Confirm ESP8266 and client are on the same network
2. Check firewall settings on network
3. Verify correct IP address in `client.py`
4. Use serial monitor to debug UDP packet reception

### Lock Not Responding

1. Check electrical connections between ESP8266 and lock
2. Verify power supply specifications match lock requirements
3. Test lock control pin output with multimeter
4. Ensure relay/driver circuit is functioning properly
5. Check lock mechanism for mechanical issues

### OLED Display Issues

1. Verify I2C connections (SDA/SCL)
2. Check power supply (3.3V)
3. Scan for I2C devices to confirm OLED address

## 📊 Monitoring

### Device Metrics

Each smart lock controller provides status information via HTTP:

```json
{
  "status": "running",
  "device": "ESP8266_SmartLock",
  "ip": "192.168.1.100",
  "ssid": "your_network",
  "rssi": -45,
  "uptime": 3600000,
  "unlock_commands_received": 15,
  "lock_status": "secure",
  "free_heap": 25000,
  "chip_id": "12345678",
  "firmware_version": "1.2.3",
  "ota_url": "https://ota.example.com/firmware",
  "ota_server_online": true
}
```

### Server Metrics

Monitor OTA server health:
- HTTP `/version` endpoint for health checks
- Kubernetes probes for availability
- Log aggregation for debugging

## 🤝 Contributing

### Getting Started

1. **Fork the ESP Smart Lock Repository**
2. **Create Feature Branch**
   ```bash
   git checkout -b feature/amazing-feature
   ```
3. **Make Changes**
   - Follow existing code style
   - Add comments for complex logic
   - Update documentation if needed
4. **Test Changes**
   - Test firmware changes on actual hardware
   - Verify OTA server functionality
   - Check CI/CD pipeline passes
5. **Submit Pull Request**
   - Describe changes clearly
   - Include testing steps
   - Reference any related issues

### Development Guidelines

- **Code Style**: Follow existing formatting and naming conventions
- **Documentation**: Update README and inline comments
- **Testing**: Test on actual hardware when possible
- **Security**: Consider security implications of changes
- **Backwards Compatibility**: Maintain compatibility when possible

## 📝 License

ESP Smart Lock is open source. Please check the license file for details.

## 🆘 Support

- **GitHub Issues**: Report bugs and request features on the ESP Smart Lock repository
- **Documentation**: Check this README and inline code comments
- **Community**: Share experiences and solutions with the ESP Smart Lock community

## 🔄 Workflow Summary

1. **Development**: Edit `sketch/sketch.ino` with new lock control features
2. **Automated Build**: GitHub Actions compiles firmware with version `1.2.{build_number}`
3. **Deployment**: Firmware automatically uploaded to OTA server
4. **Update**: ESP8266 smart lock controllers check for updates every hour and install automatically
5. **Control**: Use `client.py` to send unlock commands to doors
6. **Monitor**: Check lock status via web interface or server API

---

*ESP Smart Lock provides a complete IoT smart lock solution with automated updates, remote control, and production-ready deployment capabilities for secure access control.* 
