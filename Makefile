TARGET_EXEC := WiiUDownloader

CFLAGS := -Ofast -flto=auto -fno-fat-lto-objects -fuse-linker-plugin -pipe
LDFLAGS := -lcurl -lmbedtls -lmbedx509 -lmbedcrypto

BUILD_DIR := build
SRC_DIRS := src
INCLUDE_DIRS := include

SRCS := $(shell find $(SRC_DIRS) -name "*.c")
OBJS := $(SRCS:%=$(BUILD_DIR)/%.o)
DEPS := $(OBJS:.o=.d)
INC_DIRS := $(shell find $(INCLUDE_DIRS) -type d)
INC_FLAGS := $(addprefix -I,$(INC_DIRS))

./$(TARGET_EXEC): $(OBJS)
		gcc $(CFLAGS) $(INC_FLAGS) $(OBJS) -o $@ $(LDFLAGS)
#		strip -s $@

$(BUILD_DIR)/%.c.o: %.c
		mkdir -p $(dir $@)
		gcc $(CFLAGS) $(INC_FLAGS) -c $< -o $@

.PHONY: clean
clean:
		rm -rf $(TARGET_EXEC) $(BUILD_DIR)