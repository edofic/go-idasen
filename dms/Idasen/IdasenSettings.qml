import QtQuick
import Quickshell
import qs.Common
import qs.Widgets
import qs.Modules.Plugins

PluginSettings {
    id: root
    pluginId: "idasenDesk"

    readonly property string packagedBinaryPath: "@IDASEN_BINARY@"
    readonly property string defaultBinaryPath: packagedBinaryPath.charAt(0) === "@" ? (Quickshell.env("HOME") || "") + "/.local/bin/idasen" : packagedBinaryPath

    StyledText {
        width: parent.width
        text: "IDÅSEN Desk"
        font.pixelSize: Theme.fontSizeLarge
        font.weight: Font.Bold
        color: Theme.surfaceText
    }

    StyledText {
        width: parent.width
        text: "The widget keeps this process running to maintain one Bluetooth connection."
        wrapMode: Text.WordWrap
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.surfaceVariantText
    }

    StringSetting {
        settingKey: "binaryPath"
        label: "idasen executable"
        description: "Use an absolute path if idasen is not available on the DMS service PATH."
        placeholder: "/path/to/idasen"
        defaultValue: root.defaultBinaryPath
    }

    SliderSetting {
        settingKey: "adjustmentMm"
        label: "Fine adjustment"
        description: "Distance moved by each up/down button press."
        defaultValue: 5
        minimum: 1
        maximum: 20
        unit: " mm"
        leftIcon: "unfold_less"
        rightIcon: "unfold_more"
    }
}
