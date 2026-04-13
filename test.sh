#!/bin/bash

# Simple test script for Cryptorium
echo "=== Cryptorium Test ==="

# Check Go version
echo "1. Checking Go version..."
go version

# Run backend tests
echo -e "\n2. Running backend tests..."
cd backend
mkdir -p .gocache .ccache
GOCACHE="$PWD/.gocache" CCACHE_DIR="$PWD/.ccache" go test ./... -v

# Check frontend build
echo -e "\n3. Checking frontend build..."
cd ../frontend
if [ -d ".svelte-kit" ]; then
    echo "✅ Frontend build directory exists"
else
    echo "❌ Frontend build directory not found"
    echo "   Run 'npm run build' in frontend directory"
    exit 1
fi

# Verify configuration file
echo -e "\n4. Checking configuration..."
cd ..
if [ -f "config.yaml" ]; then
    echo "✅ Configuration file found"
else
    echo "❌ Configuration file not found"
    exit 1
fi

# Check Dockerfile
if [ -f "Dockerfile" ]; then
    echo "✅ Dockerfile found"
else
    echo "❌ Dockerfile not found"
    exit 1
fi

# Check docker-compose
if [ -f "docker-compose.yml" ]; then
    echo "✅ docker-compose.yml found"
else
    echo "❌ docker-compose.yml not found"
    exit 1
fi

# Check data directories
echo -e "\n5. Checking data directories..."
if [ -d "data" ]; then
    echo "✅ data directory exists"
else
    echo "❌ data directory not found"
fi

if [ -d "books" ]; then
    echo "✅ books directory exists"
else
    echo "❌ books directory not found"
fi

if [ -d "bookdrop" ]; then
    echo "✅ bookdrop directory exists"
else
    echo "❌ bookdrop directory not found"
fi

echo -e "\n=== Test Complete ==="
echo "Your Cryptorium setup is now ready!"
echo -e "\nNext steps:"
echo "1. Build and run with Docker: docker-compose up -d --build"
echo "2. Or run locally: cd backend && go run ./cmd/server"
echo "3. Access at http://localhost:6060"
