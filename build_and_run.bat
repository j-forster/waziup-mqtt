@echo off
set WAZIUP_API=192.168.99.100
set MONGODB=192.168.99.100
go build -o mqtt-server.exe ./cmd/  && %~dp0mqtt-server.exe
