# CryptoSSHEye - Crypto price tracker in your terminal
## Description
CryptoScope is a terminal-based TUI application to monitor live cryptocurrency prices. 
Connect via SSH and watch real-time BTC/USDT/CZK prices with dynamic colored graphs 
indicating price trends: green for rising, red for falling, and white for unchanged. 
Lightweight, fast, and fully interactive in your terminal.n

### installation
#### 1. Install go
#### 2. download packages
```go
go mod tidy
```
#### 3. run server
```go
go run mian.go
```
#### 4. connect to ssh
```go
ssh -t -p 23234 user@localhost
```
