#!/bin/bash

# 夸克网盘 CLI 工具构建脚本
# 用于为多个平台编译二进制文件

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 项目信息
PROJECT_NAME="kuake"
BUILD_DIR="dist"

# 从 main.go 中提取版本号（使用 sed 以兼容 macOS/BSD）
VERSION=$(sed -n 's/.*var Version = "\([^"]*\)".*/\1/p' cmd/main.go | head -1)
if [ -z "$VERSION" ]; then
    echo -e "${RED}Error: Failed to extract version from cmd/main.go${NC}"
    exit 1
fi

# 清理并创建输出目录
if [ -d "$BUILD_DIR" ]; then
    echo -e "${YELLOW}Cleaning ${BUILD_DIR} directory...${NC}"
    rm -rf "${BUILD_DIR}"/*
fi
mkdir -p "$BUILD_DIR"

echo -e "${GREEN}Starting build for ${PROJECT_NAME}...${NC}"
echo "Version: $VERSION (extracted from cmd/main.go)"
echo "Output directory: $BUILD_DIR"
echo ""

# 构建函数
build() {
    local os=$1
    local arch=$2
    local ext=$3
    local tags=$4  # 构建标签（debug 或空）
    
    # 构建输出文件名（包含版本号）
    local output_name="${PROJECT_NAME}-${VERSION}-${os}-${arch}${ext}"
    local output_path="${BUILD_DIR}/${output_name}"
    
    echo -e "${YELLOW}Building ${os}/${arch} (version: ${VERSION})${tags:+ (tags: $tags)}...${NC}"
    
    local build_cmd="GOOS=$os GOARCH=$arch go build -trimpath -ldflags=\"-s -w\""
    if [ -n "$tags" ]; then
        build_cmd="$build_cmd -tags=\"$tags\""
    fi
    build_cmd="$build_cmd -o \"$output_path\" ./cmd/main.go"
    
    eval $build_cmd
    
    if [ -f "$output_path" ]; then
        # 为 macOS 和 Linux 二进制文件添加执行权限
        if [ "$os" != "windows" ]; then
            chmod +x "$output_path"
        fi
        # 获取文件大小
        local size=$(du -h "$output_path" | cut -f1)
        echo -e "${GREEN}✓ Build successful: ${output_name} (${size})${NC}"
    else
        echo -e "${RED}✗ Build failed: ${output_name}${NC}"
        exit 1
    fi
    echo ""
}

# 检查 Go 环境
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go compiler not found, please install Go first${NC}"
    exit 1
fi

echo -e "${GREEN}Go version: $(go version)${NC}"
echo ""

# 构建各平台（Release 版本，不包含调试信息）
build "linux" "amd64" "" ""
build "darwin" "amd64" "" ""
build "windows" "amd64" ".exe" ""

# 创建示例配置文件到 dist 目录
echo -e "${YELLOW}Creating example config file...${NC}"
cat > "${BUILD_DIR}/config.json" << 'EOF'
{
  "Quark": {
    "access_tokens": [
      "__pus=your_pus_value_here;"
    ]
  }
}
EOF
echo -e "${GREEN}✓ Example config file created: config.json${NC}"
echo ""

echo -e "${GREEN}All platforms built successfully!${NC}"
echo ""
echo "Built files:"
ls -lh "$BUILD_DIR" | grep -E "(${PROJECT_NAME}|config)" | awk '{print "  " $9 " (" $5 ")"}'
echo ""
echo -e "${YELLOW}Usage instructions:${NC}"
echo "  Linux/macOS:"
echo "    ./dist/kuake-${VERSION}-linux-amd64 <command>"
echo "    ./dist/kuake-${VERSION}-darwin-amd64 <command>"
echo "  Windows:"
echo "    dist\\kuake-${VERSION}-windows-amd64.exe <command>"
echo ""
echo -e "${YELLOW}Note: Binary files and example config file are located in ${BUILD_DIR}/ directory${NC}"

