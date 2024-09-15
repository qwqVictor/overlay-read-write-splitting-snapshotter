# Go 编译器
GO := go

# 编译参数
BUILD_FLAGS := -v

# 目标文件名
TARGET := build/overlay-read-write-splitting-snapshotter

# 源文件列表
SOURCES := cmd/main.go

# 默认目标
all: $(TARGET)

# 编译目标
$(TARGET): $(SOURCES)
	mkdir -p build
	$(GO) build $(BUILD_FLAGS) -o $@ $(SOURCES)

# 清理目标
clean:
	rm -f $(TARGET)