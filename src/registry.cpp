#ifdef _WIN32
#include <Windows.h>
#include <iostream>
#include <registry.h>
#include <utils.h>

static bool isLongPathsEnabled() {
    HKEY hKey;
    DWORD dwValue = 0;
    DWORD dwType = REG_DWORD;
    DWORD dwSize = sizeof(dwValue);
    LONG lResult = RegOpenKeyEx(HKEY_LOCAL_MACHINE,
                                R"(SYSTEM\CurrentControlSet\Control\FileSystem)",
                                0,
                                KEY_READ,
                                &hKey);

    if (lResult == ERROR_SUCCESS) {
        lResult = RegQueryValueEx(hKey,
                                  "LongPathsEnabled",
                                  nullptr,
                                  &dwType,
                                  (LPBYTE) &dwValue,
                                  &dwSize);

        if (lResult == ERROR_SUCCESS && dwValue == 1) {
            RegCloseKey(hKey);
            return true;
        }

        RegCloseKey(hKey);
    }

    return false;
}

static bool launchExecutable(const wchar_t *executable) {
    wchar_t currentDirectory[MAX_PATH];
    GetCurrentDirectoryW(MAX_PATH, currentDirectory);

    wchar_t executablePath[MAX_PATH];
    swprintf_s(executablePath, L"%s\\%s", currentDirectory, executable);

    SHELLEXECUTEINFOW shellExecuteInfo = {sizeof(shellExecuteInfo)};
    shellExecuteInfo.lpFile = executablePath;
    shellExecuteInfo.lpDirectory = currentDirectory;
    shellExecuteInfo.nShow = SW_SHOWNORMAL;
    shellExecuteInfo.lpVerb = L"runas";

    if (ShellExecuteExW(&shellExecuteInfo)) {
        return true;
    } else {
        DWORD error = GetLastError();
        std::cout << "Failed to launch executable. Error code: " << error << std::endl;
        return false;
    }
}

void checkAndEnableLongPaths() {
    if (!isLongPathsEnabled()) {
        if (ask("Long Paths are disabled, this could cause some issues\nwhile downloading or decrypting\nEnable Long Paths?")) {
            if (launchExecutable(L"regFixLongPaths.exe"))
                showError("Successfully enabled Long Paths!");
            else
                showError("Error while enabling Long Paths!");
        }
    }
}

#endif // _WIN32