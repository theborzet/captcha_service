#!/bin/bash
set -e

PROTO_DIR="./idl/proto"
GO_OUT="./backend/pkg/api"

# Определяем путь к well-known types в зависимости от ОС
WELL_KNOWN_PATH=""

if command -v protoc &> /dev/null; then
    WELL_KNOWN_PATH=$(protoc --version | grep -q "libprotoc" && \
                      dirname $(which protoc) 2>/dev/null | sed 's|/bin$|/include|' 2>/dev/null)

    if [ ! -d "$WELL_KNOWN_PATH" ] || [ ! -f "$WELL_KNOWN_PATH/google/protobuf/timestamp.proto" ]; then
        WELL_KNOWN_PATH=""
    fi
fi

if [ -z "$WELL_KNOWN_PATH" ]; then
    echo "Не найден путь к google/protobuf/*.proto — используем fallback"
    # Fallback: скачиваем и используем локальную копию
    WELL_KNOWN_PATH="./.proto-well-known"
    mkdir -p "$WELL_KNOWN_PATH/google/protobuf"

    # Скачиваем timestamp.proto и другие нужные файлы
    curl -s -o "$WELL_KNOWN_PATH/google/protobuf/timestamp.proto" \
         "https://raw.githubusercontent.com/protocolbuffers/protobuf/main/src/google/protobuf/timestamp.proto"

    curl -s -o "$WELL_KNOWN_PATH/google/protobuf/duration.proto" \
         "https://raw.githubusercontent.com/protocolbuffers/protobuf/main/src/google/protobuf/duration.proto"

    curl -s -o "$WELL_KNOWN_PATH/google/protobuf/empty.proto" \
         "https://raw.githubusercontent.com/protocolbuffers/protobuf/main/src/google/protobuf/empty.proto"
fi

echo "Cleaning old generated code..."
rm -rf "$GO_OUT"/*

echo "Creating output directory..."
mkdir -p "$GO_OUT"

echo "Generating Go gRPC code from .proto files..."
echo "   Using well-known types from: $WELL_KNOWN_PATH"

protoc \
  --proto_path="$PROTO_DIR" \
  --proto_path="$WELL_KNOWN_PATH" \
  --go_out="$GO_OUT" \
  --go-grpc_out="$GO_OUT" \
  "$PROTO_DIR"/*.proto

echo "gRPC code generated successfully to $GO_OUT"