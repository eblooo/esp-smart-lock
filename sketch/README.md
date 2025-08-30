# ESP8266 Smart Lock Firmware

Умный замок с автоматическими OTA обновлениями и OLED дисплеем.

## 🔄 Процесс OTA обновления

### 1. Автоматическая проверка (каждые 60 сек)
```cpp
checkForUpdate() // Каждые 60 секунд в loop()
```

### 2. ESP8266 отправляет HTTP запрос
```
GET http://ota.alimov.top/firmware
Headers:
  User-Agent: ESP8266-http-Update/1.2.15
  x-esp8266-version: 1.2.15
```

### 3. OTA сервер проверяет версию
- **Если версия актуальная** → `304 Not Modified`
- **Если есть новая версия** → `200 OK` + файл прошивки

### 4. ESP8266 получает ответ
```cpp
switch (ret) {
  case HTTP_UPDATE_NO_UPDATES:  // 304 - уже актуальная
    printDisplay("FW up-to-date");
    break;
  case HTTP_UPDATE_OK:          // 200 - новая прошивка загружена
    printDisplay("OTA Success! Restarting...");
    ESP.restart();
    break;
  case HTTP_UPDATE_FAILED:      // Ошибка загрузки
    printDisplay("OTA Failed");
    break;
}
```

### 5. Процесс установки (при UPDATE_OK)
1. ESP8266 автоматически записывает новую прошивку во flash
2. Проверяет MD5 хэш для целостности
3. Перезагружается с новой версией
4. Новая версия отображается на дисплее

## 📡 Коммуникация

### UDP команды (порт 2333)
```bash
echo "unlock" | nc -u <ESP_IP> 2333  # Разблокировать
echo "lock" | nc -u <ESP_IP> 2333    # Заблокировать
```

### HTTP статус (порт 80)
```bash
curl http://<ESP_IP>/          # Получить статус JSON
curl -X POST http://<ESP_IP>/update  # Принудительная проверка OTA
```

## 📺 OLED Дисплей
```
Locked | FW:1.2.15   O    # Статус + версия + анимация
IP: 192.168.1.100           # IP адрес
Unlocks: 5                  # Счетчик разблокировок
Uptime: 3600s               # Время работы
```

## ⚙️ Конфигурация

```cpp
const char* firmwareUrl = "http://ota.alimov.top/firmware";
const unsigned long updateInterval = 60000; // 60 сек
const unsigned int localPort = 2333;        // UDP порт
```

## 🔨 Сборка

Версия прошивки автоматически устанавливается в CI/CD:
```cpp
#define FIRMWARE_VERSION "%%FIRMWARE_VERSION%%"  // → 1.2.15
```

При сборке GitHub Actions заменяет плейсхолдер на реальную версию из `git describe`.
