#include <ESP8266WiFi.h>
#include <WiFiUdp.h>
#include <U8g2lib.h>
#include <ESP8266httpUpdate.h>
#include <ESP8266WebServer.h>
#include <WiFiClientSecureBearSSL.h>

// WiFi credentials (set via CI/CD)
const char* ssid = "YOUR_WIFI_SSID";      // WiFi SSID (replace via CI/CD)
const char* password = "YOUR_WIFI_PASSWORD"; // WiFi password (replace via CI/CD)

// UDP settings
const unsigned int localPort = 2333; // Port to listen on
WiFiUDP udp;

// OTA settings
const char* firmwareUrl = "http://ota.alimov.top/firmware"; // OTA server endpoint (HTTP for ESP8266 compatibility)
const char* otaServerHost = "ota.alimov.top"; // OTA server host for connectivity testing
const unsigned long updateInterval = 60000; // Check every 30 seconds for testing (revert to 3600000 for 1 hour)
WiFiClient client; // Use WiFiClient for HTTP connections
unsigned long lastUpdateCheck = 0;

// Simple web server for status and manual updates only
ESP8266WebServer server(80);

// OLED display initialization (SCL=D6=GPIO12, SDA=D5=GPIO14)
U8G2_SSD1306_128X64_NONAME_F_SW_I2C display(U8G2_R0, /* clock=*/12, /* data=*/14, /* reset=*/U8X8_PIN_NONE);

// LED pin
const int LED_PIN = 4; // D2 = GPIO 4

// Buffer for incoming messages
char packetBuffer[255];

// Packet counter
unsigned int packetCount = 0;

void printDisplay(const char* msg1, const char* msg2) {
  display.clearBuffer();
  display.setFont(u8g2_font_ncenB08_tr);
  display.drawStr(0, 10, msg1);
  display.drawStr(0, 25, msg2);
  display.sendBuffer();
}

void logRequest(const String& method, const String& uri, const String& clientIP) {
  Serial.printf("[%lu] %s %s from %s\n", millis(), method.c_str(), uri.c_str(), clientIP.c_str());
}

void reconnectWiFi() {
  if (WiFi.status() != WL_CONNECTED) {
    Serial.println("WiFi disconnected, attempting to reconnect...");
    printDisplay("Reconnecting...", "");
    WiFi.begin(ssid, password);
    int attempts = 0;
    while (WiFi.status() != WL_CONNECTED && attempts < 20) {
      delay(500);
      Serial.print(".");
      attempts++;
    }
    if (WiFi.status() == WL_CONNECTED) {
      Serial.println("\nReconnected to WiFi");
      printDisplay("WiFi Reconnected", WiFi.localIP().toString().c_str());
      udp.begin(localPort); // Restart UDP
      server.begin(); // Restart web server
    } else {
      Serial.println("\nReconnection failed");
      printDisplay("WiFi Failed", ("Packets: " + String(packetCount)).c_str());
    }
  }
}

bool testServerConnectivity() {
  Serial.printf("Testing connectivity to OTA server %s...\n", otaServerHost);
  
  WiFiClient testClient;
  testClient.setTimeout(5000); // 5 second timeout
  
  if (testClient.connect(otaServerHost, 443)) {
    Serial.println("✓ Server connectivity test passed");
    testClient.stop();
    return true;
  } else {
    Serial.println("✗ Server connectivity test failed");
    return false;
  }
}

void checkForUpdate() {
  Serial.println("Checking for firmware update...");
  printDisplay("Checking OTA", ("Packets: " + String(packetCount)).c_str());

  // First test basic connectivity
  if (!testServerConnectivity()) {
    Serial.println("Cannot reach OTA server, skipping update check");
    printDisplay("OTA Server", "Unreachable");
    return;
  }

  // Set timeout for OTA client
  client.setTimeout(10000); // 10 second timeout

  Serial.println("Requesting OTA update from " + String(firmwareUrl));
  
  // Set current firmware version lower than server's latest to force update
  const char* currentVersion = "%%FIRMWARE_VERSION%%";
  
  // Use update with version checking (no custom headers)
  t_httpUpdate_return ret = ESPhttpUpdate.update(client, firmwareUrl, currentVersion);

  // Output details to serial and display
  Serial.printf("Update result: %d\n", ret);
  
  switch (ret) {
    case HTTP_UPDATE_OK:
      Serial.println("✅ Update successful, restarting...");
      printDisplay("OTA Success", "Restarting...");
      delay(1000);
      ESP.restart();
      break;
      
    case HTTP_UPDATE_NO_UPDATES:
      Serial.println("✅ No update available (firmware is current)");
      printDisplay("Firmware Current", ("Packets: " + String(packetCount)).c_str());
      break;
      
    case HTTP_UPDATE_FAILED:
      String error = ESPhttpUpdate.getLastErrorString();
      int errorCode = ESPhttpUpdate.getLastError();
      Serial.printf("✗ Update failed (code: %d): %s\n", errorCode, error.c_str());
      
      // More specific error handling
      if (errorCode == -104) {
        Serial.println("HTTP_UPDATE_FAILED: Wrong HTTP Code - server returned unexpected status");
        printDisplay("HTTP Error", "Wrong Code");
      } else if (error.indexOf("connection") >= 0) {
        printDisplay("Connection Failed", "Check Network");
      } else if (error.indexOf("HTTP") >= 0) {
        printDisplay("HTTP Error", ("Code: " + String(errorCode)).c_str());
      } else if (error.indexOf("Verify") >= 0) {
        printDisplay("Invalid Firmware", "Wrong Binary");
      } else {
        printDisplay("OTA Failed", error.c_str());
      }
      break;
  }
}

void setup() {
  // Start serial for debugging
  Serial.begin(115200);
  delay(1000);
  
  // Print basic memory information
  Serial.println("\n=== ESP8266 Memory Info ===");
  Serial.printf("Free heap: %u bytes\n", ESP.getFreeHeap());
  Serial.printf("Sketch size: %u bytes\n", ESP.getSketchSize());
  Serial.printf("Free sketch space: %u bytes\n", ESP.getFreeSketchSpace());
  Serial.printf("Flash chip size: %u bytes\n", ESP.getFlashChipRealSize());
  Serial.println("===========================\n");

  // Initialize LED pin
  pinMode(LED_PIN, OUTPUT);
  digitalWrite(LED_PIN, HIGH); // Start with LED off

  // Initialize OLED
  display.begin();
  printDisplay("Connecting...", "");

  // Connect to WiFi
  WiFi.begin(ssid, password);
  while (WiFi.status() != WL_CONNECTED) {
    delay(500);
    Serial.print(".");
    printDisplay("Connecting...", "");
  }
  Serial.println("\nConnected to WiFi");
  Serial.println(WiFi.localIP());

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
    response += "  \"firmware_version\": \"1.0.0\",\n";
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
      printDisplay("T - ON", ("Packets: " + String(packetCount)).c_str());
      delay(500); // Brief LED on duration
      digitalWrite(LED_PIN, HIGH);
      printDisplay("T - OFF - Sasha", ("Packets: " + String(packetCount)).c_str());
    } else if (strcmp(packetBuffer, "f_pressed") == 0) {
      digitalWrite(LED_PIN, LOW); // Turn LED on
      printDisplay("F Pressed", ("Packets: " + String(packetCount)).c_str());
      delay(500); // Brief LED on duration
      digitalWrite(LED_PIN, HIGH);
      printDisplay("F - OFF", ("Packets: " + String(packetCount)).c_str());
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
