// --- ESP Smart Lock Firmware ---
#include <ESP8266WiFi.h>           // WiFi support
#include <WiFiUdp.h>               // UDP for lock commands
#include <U8g2lib.h>               // OLED display
#include <ESP8266httpUpdate.h>     // OTA updates
#include <ESP8266WebServer.h>      // Simple web server

// --- Configuration ---
#define FIRMWARE_VERSION "%%FIRMWARE_VERSION%%"  // Will be replaced by CI/CD build process
const char* ssid = "YOUR_WIFI_SSID";      // WiFi SSID
const char* password = "YOUR_WIFI_PASSWORD"; // WiFi password
const unsigned int localPort = 2333;       // UDP port
const char* firmwareUrl = "http://ota.alimov.top/firmware"; // OTA server
const unsigned long updateInterval = 60000; // OTA check interval (ms)

// --- Globals ---
WiFiUDP udp;
ESP8266WebServer server(80);
U8G2_SSD1306_128X64_NONAME_F_SW_I2C display(U8G2_R0, /* clock=*/12, /* data=*/14, /* reset=*/U8X8_PIN_NONE);
const int LED_PIN = 4; // D2 = GPIO 4
char packetBuffer[255];
unsigned long lastUpdateCheck = 0;
String lockStatus = "Locked"; // Always display lock status on main line
unsigned int unlockCount = 0; // Counter for unlock commands received

// --- Animation variables ---
unsigned long lastAnimationUpdate = 0;
int animationFrame = 0;
const char* aliveIndicator[] = {".", "o", "O", "o"}; // Growing/shrinking circle animation
const int animationSpeed = 300; // Update every 300ms for smooth breathing effect

// --- Display Helper ---
// Line 1: lock status + firmware version + heartbeat, Line 2: IP, Line 3: unlock count, Line 4: uptime, Line 5: dynamic info
void printDisplay(const String& dynamicInfo = "") {
  // Update animation frame
  unsigned long now = millis();
  if (now - lastAnimationUpdate >= animationSpeed) {
    animationFrame = (animationFrame + 1) % 4;
    lastAnimationUpdate = now;
  }
  
  display.clearBuffer();
  display.setFont(u8g2_font_ncenB08_tr);
  
  // Line 1: Status + Version + Breathing Animation (with spaces before indicator)
  String line1 = lockStatus + " | FW:" + String(FIRMWARE_VERSION) + "   " + aliveIndicator[animationFrame];
  display.drawStr(0, 10, line1.c_str());
  
  display.drawStr(0, 25, ("IP: " + WiFi.localIP().toString()).c_str());
  display.drawStr(0, 37, ("Unlocks: " + String(unlockCount)).c_str());
  display.drawStr(0, 49, ("Uptime: " + String(millis() / 1000) + "s").c_str());
  if (dynamicInfo.length() > 0) {
    display.drawStr(0, 61, dynamicInfo.c_str());
  }
  display.sendBuffer();
}

// --- WiFi Reconnect ---
void reconnectWiFi() {
  if (WiFi.status() != WL_CONNECTED) {
    printDisplay("WiFi reconnecting...");
    WiFi.begin(ssid, password);
    int attempts = 0;
    while (WiFi.status() != WL_CONNECTED && attempts < 20) {
      delay(500);
      attempts++;
    }
    if (WiFi.status() == WL_CONNECTED) {
      printDisplay("WiFi OK");
      udp.begin(localPort);
      server.begin();
    } else {
      printDisplay("WiFi Failed");
    }
  }
}

// --- OTA Update ---
void checkForUpdate() {
  WiFiClient client;
  printDisplay("Checking OTA...");
  client.setTimeout(10000);
  t_httpUpdate_return ret = ESPhttpUpdate.update(client, firmwareUrl, FIRMWARE_VERSION);
  switch (ret) {
    case HTTP_UPDATE_OK:
      printDisplay("OTA Success! Restarting...");
      delay(1000);
      ESP.restart();
      break;
    case HTTP_UPDATE_NO_UPDATES:
      printDisplay("FW up-to-date");
      break;
    case HTTP_UPDATE_FAILED:
      printDisplay("OTA Failed");
      break;
  }
}

// --- Setup ---
void setup() {
  Serial.begin(115200);
  pinMode(LED_PIN, OUTPUT);
  digitalWrite(LED_PIN, HIGH); // Locked by default
  display.begin();
  printDisplay("Connecting...");
  WiFi.begin(ssid, password);
  while (WiFi.status() != WL_CONNECTED) {
    delay(500);
    printDisplay("Connecting...");
  }
  udp.begin(localPort);
  printDisplay("Ready");
  // Web server: status and manual OTA
  server.on("/", HTTP_GET, []() {
    String response = "{\n";
    response += "  \"status\": \"running\",\n";
    response += "  \"device\": \"ESP8266\",\n";
    response += "  \"ip\": \"" + WiFi.localIP().toString() + "\",\n";
    response += "  \"firmware_version\": \"" + String(FIRMWARE_VERSION) + "\",\n";
    response += "  \"lock_status\": \"" + lockStatus + "\",\n";
    response += "  \"unlock_count\": " + String(unlockCount) + ",\n";
    response += "  \"uptime_seconds\": " + String(millis() / 1000) + ",\n";
    response += "  \"uptime_formatted\": \"" + String(millis() / 1000) + "s\",\n";
    response += "  \"free_heap\": " + String(ESP.getFreeHeap()) + ",\n";
    response += "  \"wifi_rssi\": " + String(WiFi.RSSI()) + ",\n";
    response += "  \"chip_id\": \"" + String(ESP.getChipId()) + "\",\n";
    response += "  \"animation_frame\": " + String(animationFrame) + "\n";
    response += "}";
    server.send(200, "application/json", response);
  });
  server.on("/update", HTTP_POST, []() {
    printDisplay("Manual OTA...");
    server.send(200, "text/plain", "OTA update triggered");
    delay(500);
    checkForUpdate();
  });
  server.begin();
}

// --- Main Loop ---
void loop() {
  server.handleClient();
  
  // Update display animation regularly
  static unsigned long lastDisplayUpdate = 0;
  unsigned long now = millis();
  if (now - lastDisplayUpdate >= 100) { // Update display every 100ms
    printDisplay();
    lastDisplayUpdate = now;
  }
  
  int packetSize = udp.parsePacket();
  if (packetSize) {
    int len = udp.read(packetBuffer, 255);
    if (len > 0) packetBuffer[len] = 0;
    if (strcmp(packetBuffer, "unlock") == 0) {
      digitalWrite(LED_PIN, LOW); // Unlock
      lockStatus = "Unlocked";
      unlockCount++; // Increment unlock counter 
      printDisplay();
      delay(100);
      digitalWrite(LED_PIN, HIGH); // Lock again
      lockStatus = "Locked";
      printDisplay();
    } else if (strcmp(packetBuffer, "lock") == 0) {
      digitalWrite(LED_PIN, HIGH); // Lock
      lockStatus = "Locked";
      printDisplay();
    }
  }
  
  if (now - lastUpdateCheck >= updateInterval || now < lastUpdateCheck) {
    reconnectWiFi();
    if (WiFi.status() == WL_CONNECTED) {
      checkForUpdate();
      lastUpdateCheck = now;
    }
  }
}
