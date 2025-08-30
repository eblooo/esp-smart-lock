#include <ESP8266WiFi.h>

// --- ESP Smart Lock Firmware ---
#include <ESP8266WiFi.h>           // WiFi support
#include <WiFiUdp.h>               // UDP for lock commands
#include <U8g2lib.h>               // OLED display
#include <ESP8266httpUpdate.h>     // OTA updates
#include <ESP8266WebServer.h>      // Simple web server

// --- Configuration ---
#define FIRMWARE_VERSION "1.0.0"
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

// --- Display Helper ---
// Fixed first 3 lines: lock status, device IP, unlock count. Line 4: dynamic info (OTA, etc)
void printDisplay(const String& dynamicInfo = "") {
  display.clearBuffer();
  display.setFont(u8g2_font_ncenB08_tr);
  display.drawStr(0, 10, ("Lock: " + lockStatus).c_str());
  display.drawStr(0, 20, ("IP: " + WiFi.localIP().toString()).c_str());
  display.drawStr(0, 30, ("Unlocks: " + String(unlockCount)).c_str());
  if (dynamicInfo.length() > 0) {
    display.drawStr(0, 40, dynamicInfo.c_str());
  } else {
    display.drawStr(0, 40, ("FW: " FIRMWARE_VERSION).c_str());
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
      printDisplay("Firmware Up-to-date");
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
    response += "  \"unlock_count\": " + String(unlockCount) + "\n";
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
  int packetSize = udp.parsePacket();
  if (packetSize) {
    int len = udp.read(packetBuffer, 255);
    if (len > 0) packetBuffer[len] = 0;
    if (strcmp(packetBuffer, "unlock") == 0) {
      digitalWrite(LED_PIN, LOW); // Unlock
      lockStatus = "Unlocked";
      unlockCount++; // Increment unlock counter
      printDisplay();
      delay(500);
      digitalWrite(LED_PIN, HIGH); // Lock again
      lockStatus = "Locked";
      printDisplay();
    } else if (strcmp(packetBuffer, "lock") == 0) {
      digitalWrite(LED_PIN, HIGH); // Lock
      lockStatus = "Locked";
      printDisplay();
    }
  }
  unsigned long now = millis();
  if (now - lastUpdateCheck >= updateInterval || now < lastUpdateCheck) {
    reconnectWiFi();
    if (WiFi.status() == WL_CONNECTED) {
      checkForUpdate();
      lastUpdateCheck = now;
    }
  }
}
  // Start UDP
  udp.begin(localPort);
  printDisplay("UDP Started", "");

  // Start simple web server (status and manual OTA only)
  // Add status endpoint
  server.on("/", HTTP_GET, []() {
    logRequest("GET", "/", server.client().remoteIP().toString());
    
    // Test OTA server connectivity for status
    bool otaServerOnline = testServerConnectivity();
    
    String response = "{\n";
    response += "  \"status\": \"running\",\n";
    response += "  \"device\": \"ESP8266\",\n";
    response += "  \"ip\": \"" + WiFi.localIP().toString() + "\",\n";
    response += "  \"ssid\": \"" + String(ssid) + "\",\n";
    response += "  \"rssi\": " + String(WiFi.RSSI()) + ",\n";
    response += "  \"uptime\": " + String(millis()) + ",\n";
    response += "  \"packets_received\": " + String(packetCount) + ",\n";
    response += "  \"free_heap\": " + String(ESP.getFreeHeap()) + ",\n";
    response += "  \"chip_id\": \"" + String(ESP.getChipId()) + "\",\n";
  response += "  \"firmware_version\": \"" + String(FIRMWARE_VERSION) + "\",\n";
    response += "  \"ota_url\": \"" + String(firmwareUrl) + "\",\n";
    response += "  \"ota_server_online\": " + String(otaServerOnline ? "true" : "false") + "\n";
    response += "}";
    server.send(200, "application/json", response);
  });
  
  // Add manual OTA trigger endpoint
  server.on("/update", HTTP_POST, []() {
    logRequest("POST", "/update", server.client().remoteIP().toString());
    printDisplay("Manual Update", "Triggered");
    server.send(200, "text/plain", "OTA update triggered - checking server for new firmware");
    delay(500);
    checkForUpdate();
  });
  
  // Add catch-all handler for logging unhandled requests
  server.onNotFound([]() {
    logRequest(server.method() == HTTP_GET ? "GET" : 
               server.method() == HTTP_POST ? "POST" : 
               server.method() == HTTP_PUT ? "PUT" : 
               server.method() == HTTP_DELETE ? "DELETE" : "OTHER", 
               server.uri(), server.client().remoteIP().toString());
    server.send(404, "text/plain", "Not Found");
  });
  
  server.begin();
  Serial.println("Web server started");

  // Convert to string and display IP address
  String ipAddress = WiFi.localIP().toString();
  printDisplay("IP Address:", ipAddress.c_str());
}

void loop() {
  // Handle web server
  server.handleClient();

  // Check for incoming UDP packets
  int packetSize = udp.parsePacket();
  if (packetSize) {
    // Increment packet counter
    packetCount++;

    // Read the packet into the buffer
    int len = udp.read(packetBuffer, 255);
    if (len > 0) {
      packetBuffer[len] = 0; // Null-terminate the string
    }

    Serial.printf("Received: %s\n", packetBuffer);

    // Update OLED and LED based on received signal
    if (strcmp(packetBuffer, "unlock") == 0) {
      digitalWrite(LED_PIN, LOW); // Turn LED on
  printDisplay("Unlocked", ("Packets: " + String(packetCount)).c_str());
      delay(500); // Brief LED on duration
      digitalWrite(LED_PIN, HIGH);
  printDisplay("Locked", ("Packets: " + String(packetCount)).c_str());
    } else if (strcmp(packetBuffer, "f_pressed") == 0) {
      digitalWrite(LED_PIN, LOW); // Turn LED on
  printDisplay("F Pressed", ("Packets: " + String(packetCount)).c_str());
      delay(500); // Brief LED on duration
      digitalWrite(LED_PIN, HIGH);
  printDisplay("F Released", ("Packets: " + String(packetCount)).c_str());
    } else {
  printDisplay("Unknown Signal", ("Packets: " + String(packetCount)).c_str());
    }
  }

  // Check for OTA updates periodically
  unsigned long now = millis();
  //Serial.printf("Checking OTA timer: now=%lu, lastUpdateCheck=%lu, diff=%lu\n", now, lastUpdateCheck, now - lastUpdateCheck);
  if (now - lastUpdateCheck >= updateInterval || now < lastUpdateCheck) {
    reconnectWiFi();
    if (WiFi.status() == WL_CONNECTED) {
      Serial.println("Triggering OTA check...");
      delay(random(0, 5000));
      checkForUpdate();
      lastUpdateCheck = now;
    } else {
      Serial.println("WiFi disconnected, skipping update check");
  printDisplay("WiFi Lost", ("Packets: " + String(packetCount)).c_str());
    }
  }
}
