# TCP Reverse Tunnel

Обратный TCP-туннель.

Принцип работы:
  - Хост при старте пытается подключиться к серверу. В случае успеха, держит соединение открытым.
  - `yamux` мультиплексирует туннель. Делает из одного TCP-соединения множество независимых логических каналов (стримов).
  - Когда приходит внешний клиент: сервер открывает новый стрим внутри уже существующего туннеля.
  - Хост видит новый стрим, подключается к локальному сервису и проксирует данные.

Для внешнего клиента всё выглядит как обычное TCP-соединение. Он не знает ни о туннеле, ни о хосте.

## Keygen

```bash
mkdir -p tls/ \
  && openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout tls/tunnel.key -out tls/tunnel.crt \
    -subj "/CN=localhost" \
    -addext "subjectAltName = IP:127.0.0.1,DNS:localhost" \
  && chmod 777 tls/*
```

### Server

  * `tunnel` - 0.0.0.0:port Публичный порт для подключения хоста.
  * `listen` - Локальный host:port.
  * `tls` - TLS подключение.
  * `cert` - Путь к сертификату.
  * `key` - Путь к приватному ключу.

**With TLS**
```bash
./server \
  --tunnel 0.0.0.0:10100 \
  --listen 127.0.0.1:7000 \
  --tls \
  --cert tls/tunnel.crt \
  --key tls/tunnel.key
```

**Without TLS**
```bash
./server \
  --listen 0.0.0.0:10100 \
  --local 127.0.0.1:7000
```

### Client

  * `server` - host:port туннель доступный из интернета.
  * `forward` - host:port локальный сервер.
  * `tls` - TLS подключение.
  * `tls-skip-verify` - Не проверять сертификат сервера.


**With TLS**
```bash
./host \
  --server 31.31.207.164:10100 \
  --forward 127.0.0.1:3000 \
  --tls \
  --tls-skip-verify
```

**Without TLS**
```bash
./host \
  --server 31.31.207.164:10100 \
  --forward 127.0.0.1:3000
```
