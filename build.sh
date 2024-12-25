#!/bin/bash

APP_NAME=sentinel-sync-dns

mkdir -p bin

rm -f bin/$APP_NAME.zip
rm -f bin/$APP_NAME
rm -f bin/${APP_NAME}.exe

echo "Building for Linux..."
GOOS=linux GOARCH=amd64 go build -o bin/$APP_NAME

echo "Building for Windows..."
GOOS=windows GOARCH=amd64 go build -o bin/${APP_NAME}.exe

echo "Creating zip archive..."
cd bin
zip $APP_NAME.zip $APP_NAME ${APP_NAME}.exe
cd ..

rm -f bin/$APP_NAME
rm -f bin/${APP_NAME}.exe

echo "Build complete!"