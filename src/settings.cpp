#include <settings.h>

#include <downloader.h>
#include <json/json.h>

#include <fstream>

static void utf8JSON(std::fstream &file, const Json::Value &value) {
    Json::StreamWriterBuilder builder;
    builder.settings_["emitUTF8"] = true;
    std::unique_ptr<Json::StreamWriter> writer(builder.newStreamWriter());
    writer->write(value, &file);
}

bool saveSettings(const char *selectedDir, bool hideWiiVCWarning) {
    Json::Value root;
    if (selectedDir != nullptr)
        root["downloadDir"] = selectedDir;
    else if (getSelectedDir() != nullptr)
        root["downloadDir"] = getSelectedDir();
    else
        root["downloadDir"] = "";
    root["hideWiiVCWarning"] = hideWiiVCWarning;

    std::fstream settingsFile("settings.json", std::fstream::out);
    if (!settingsFile.is_open())
        return false;
    utf8JSON(settingsFile, root);
    settingsFile.close();
    setSelectedDir(selectedDir);
    return true;
}

bool loadSettings() {
    Json::Value root;
    std::fstream settingsFile("settings.json", std::fstream::in);
    if (!settingsFile.is_open())
        return false;
    try {
        settingsFile >> root;
    } catch (const Json::RuntimeError::exception &e) {
        root["downloadDir"] = "";
        root["hideWiiVCWarning"] = false;
    }
    settingsFile.close();
    if (root.empty())
        return false;
    if (!root["downloadDir"].empty())
        setSelectedDir(root["downloadDir"].asCString());
    setHideWiiVCWarning(root["hideWiiVCWarning"].asBool());
    return true;
}