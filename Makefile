TARGET_EXEC := WiiUDownloader

CFLAGS := -std=c99 -D_GNU_SOURCE -UNDEBUG -DAES_ROM_TABLES -fvisibility=hidden -Ofast -fno-strict-aliasing -pipe `pkg-config gtkmm-3.0 --cflags` -I/usr/local/include
CXXFLAGS := $(CFLAGS) -fpermissive -std=c++20
LDFLAGS := -L/usr/local/lib -lmbedtls -lmbedx509 -lmbedcrypto `pkg-config gtkmm-3.0 libcurl --libs`

BUILD_DIR := build
SRC_DIRS := src
INCLUDE_DIRS := include

SRCS := $(shell find $(SRC_DIRS) -name "*.cpp")
SRCS += $(shell find $(SRC_DIRS) -name "*.c")
OBJS := $(SRCS:%=$(BUILD_DIR)/%.o)
DEPS := $(OBJS:.o=.d)
INC_DIRS := $(shell find $(INCLUDE_DIRS) -type d)
INC_FLAGS := $(addprefix -I,$(INC_DIRS))

./$(TARGET_EXEC): $(OBJS)
	g++ $(CXXFLAGS) $(INC_FLAGS) $(OBJS) -o $@ $(LDFLAGS)
#	strip -s $@

$(BUILD_DIR)/%.cpp.o: %.cpp
	mkdir -p $(dir $@)
	g++ $(CXXFLAGS) $(INC_FLAGS) -c $< -o $@

$(BUILD_DIR)/%.c.o: %.c
	mkdir -p $(dir $@)
	gcc $(CFLAGS) $(INC_FLAGS) -c $< -o $@
	

.PHONY: clean
clean:
	rm -rf $(TARGET_EXEC) $(BUILD_DIR)