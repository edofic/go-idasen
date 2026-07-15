import QtQuick
import Quickshell
import Quickshell.Io
import qs.Common
import qs.Widgets
import qs.Modules.Plugins

PluginComponent {
    id: root

    layerNamespacePlugin: "idasen-desk"

    property real deskHeight: 0
    property real deskSpeed: 0
    property string controllerStatus: "connecting"
    property string errorMessage: ""
    property var positions: ({})
    readonly property string packagedBinaryPath: "@IDASEN_BINARY@"
    readonly property string defaultBinaryPath: (Quickshell.env("HOME") || "") + "/.local/bin/idasen"
    property string binaryPath: pluginData.binaryPath || (packagedBinaryPath.charAt(0) === "@" ? defaultBinaryPath : packagedBinaryPath)
    property int adjustmentMm: pluginData.adjustmentMm || 5
    property bool shuttingDown: false

    readonly property bool connected: deskHeight > 0 && controllerStatus !== "unavailable" && controllerStatus !== "disconnected"
    readonly property string heightText: connected ? (deskHeight * 100).toFixed(1) + " cm" : "--.- cm"

    function send(command) {
        if (!controllerProcess.running) {
            showError("Desk controller is not running");
            return;
        }
        controllerProcess.write(JSON.stringify(command) + "\n");
    }

    function showError(message) {
        errorMessage = message;
        errorTimer.restart();
    }

    function moveToPosition(name) {
        send({"action": "move", "position": name});
    }

    function adjust(direction) {
        send({"action": "adjust", "delta": direction * adjustmentMm / 1000.0});
    }

    function savePosition(name, centimetres) {
        const parsed = Number(centimetres);
        if (!Number.isFinite(parsed) || parsed < 62 || parsed > 127) {
            showError("Height must be between 62 and 127 cm");
            return false;
        }
        send({"action": "set_position", "position": name, "height": parsed / 100.0});
        return true;
    }

    function positionText(name) {
        const value = positions[name];
        return value === undefined ? "" : (value * 100).toFixed(1);
    }

    function handleEvent(line) {
        if (!line.trim())
            return;
        try {
            const event = JSON.parse(line);
            const wasDisconnected = controllerStatus === "connecting" || controllerStatus === "unavailable" || controllerStatus === "disconnected";
            if (event.status)
                controllerStatus = event.status;
            if (event.type === "height") {
                deskHeight = event.height;
                deskSpeed = event.speed || 0;
                if (wasDisconnected)
                    errorMessage = "";
            } else if (event.type === "positions") {
                positions = event.positions || {};
                errorMessage = "";
            } else if (event.type === "status" && event.positions) {
                positions = event.positions;
            } else if (event.type === "error") {
                showError(event.message || "Unknown desk error");
            }
        } catch (error) {
            console.warn("IDÅSEN: invalid controller event:", line, error);
        }
    }

    Process {
        id: controllerProcess
        command: [root.binaryPath, "controller"]
        stdinEnabled: true
        running: false

        stdout: SplitParser {
            onRead: line => root.handleEvent(line)
        }

        stderr: SplitParser {
            onRead: line => {
                if (line.trim())
                    console.warn("IDÅSEN:", line);
            }
        }

        onExited: (exitCode, exitStatus) => {
            root.controllerStatus = "disconnected";
            if (!root.shuttingDown) {
                if (!root.errorMessage)
                    root.showError("Desk controller exited (code " + exitCode + ")");
                restartTimer.restart();
            }
        }
    }

    Timer {
        id: restartTimer
        interval: 5000
        repeat: false
        onTriggered: {
            root.controllerStatus = "connecting";
            controllerProcess.running = true;
        }
    }

    Timer {
        id: errorTimer
        interval: 6000
        repeat: false
        onTriggered: root.errorMessage = ""
    }

    Component.onCompleted: controllerProcess.running = true
    Component.onDestruction: {
        shuttingDown = true;
        controllerProcess.running = false;
    }

    horizontalBarPill: Component {
        Row {
            spacing: Theme.spacingS

            DankIcon {
                name: root.controllerStatus === "moving" ? "height" : "desk"
                size: root.iconSize
                color: root.errorMessage ? Theme.error : Theme.primary
                anchors.verticalCenter: parent.verticalCenter
            }

            StyledText {
                text: root.heightText
                font.pixelSize: Theme.fontSizeMedium
                color: Theme.surfaceText
                anchors.verticalCenter: parent.verticalCenter
            }
        }
    }

    verticalBarPill: Component {
        Column {
            spacing: 0

            DankIcon {
                name: "desk"
                size: root.iconSize
                color: root.errorMessage ? Theme.error : Theme.primary
                anchors.horizontalCenter: parent.horizontalCenter
            }

            StyledText {
                text: root.connected ? Math.round(root.deskHeight * 100) : "--"
                font.pixelSize: Theme.fontSizeSmall
                color: Theme.surfaceText
                anchors.horizontalCenter: parent.horizontalCenter
            }
        }
    }

    popoutContent: Component {
        PopoutComponent {
            id: popout

            headerText: "IDÅSEN Desk"
            detailsText: root.controllerStatus === "moving" ? "Desk is moving" : (root.connected ? "Live height" : "Connecting to desk…")
            showCloseButton: true

            Column {
                width: parent.width
                spacing: Theme.spacingM
                topPadding: Theme.spacingM
                leftPadding: Theme.spacingS
                rightPadding: Theme.spacingS

                StyledText {
                    width: parent.width - Theme.spacingS * 2
                    text: root.heightText
                    horizontalAlignment: Text.AlignHCenter
                    font.pixelSize: Theme.fontSizeXLarge + 12
                    font.weight: Font.Bold
                    color: root.errorMessage ? Theme.error : Theme.primary
                }

                StyledText {
                    width: parent.width - Theme.spacingS * 2
                    visible: root.errorMessage !== ""
                    text: root.errorMessage
                    horizontalAlignment: Text.AlignHCenter
                    wrapMode: Text.WordWrap
                    font.pixelSize: Theme.fontSizeSmall
                    color: Theme.error
                }

                Row {
                    width: parent.width - Theme.spacingS * 2
                    spacing: Theme.spacingS

                    DankButton {
                        width: (parent.width - parent.spacing) / 2
                        text: "Sit" + (root.positions.sit === undefined ? "" : "  ·  " + Math.round(root.positions.sit * 100) + " cm")
                        iconName: "chair"
                        enabled: root.connected && root.positions.sit !== undefined
                        onClicked: root.moveToPosition("sit")
                    }

                    DankButton {
                        width: (parent.width - parent.spacing) / 2
                        text: "Stand" + (root.positions.stand === undefined ? "" : "  ·  " + Math.round(root.positions.stand * 100) + " cm")
                        iconName: "accessibility_new"
                        enabled: root.connected && root.positions.stand !== undefined
                        onClicked: root.moveToPosition("stand")
                    }
                }

                Row {
                    width: parent.width - Theme.spacingS * 2
                    spacing: Theme.spacingS

                    DankButton {
                        width: (parent.width - parent.spacing * 2) / 3
                        text: "Down"
                        iconName: "arrow_downward"
                        enabled: root.connected
                        onClicked: root.adjust(-1)
                    }

                    DankButton {
                        width: (parent.width - parent.spacing * 2) / 3
                        text: "Stop"
                        iconName: "stop"
                        enabled: root.connected
                        backgroundColor: Theme.surfaceContainerHighest
                        textColor: Theme.surfaceText
                        onClicked: root.send({"action": "stop"})
                    }

                    DankButton {
                        width: (parent.width - parent.spacing * 2) / 3
                        text: "Up"
                        iconName: "arrow_upward"
                        enabled: root.connected
                        onClicked: root.adjust(1)
                    }
                }

                StyledText {
                    width: parent.width - Theme.spacingS * 2
                    text: "Fine adjustment: " + root.adjustmentMm + " mm"
                    horizontalAlignment: Text.AlignHCenter
                    font.pixelSize: Theme.fontSizeSmall
                    color: Theme.surfaceVariantText
                }

                Rectangle {
                    width: parent.width - Theme.spacingS * 2
                    height: 1
                    color: Theme.outlineVariant
                }

                PositionEditor {
                    width: parent.width - Theme.spacingS * 2
                    positionName: "sit"
                    label: "Sit height"
                }

                PositionEditor {
                    width: parent.width - Theme.spacingS * 2
                    positionName: "stand"
                    label: "Stand height"
                }
            }
        }
    }

    component PositionEditor: Row {
        id: editor
        required property string positionName
        required property string label
        spacing: Theme.spacingS

        DankTextField {
            id: heightField
            width: parent.width - saveButton.width - parent.spacing
            labelText: editor.label + " (cm)"
            placeholderText: "62–127"
            text: root.positionText(editor.positionName)
            validator: DoubleValidator {
                bottom: 62
                top: 127
                decimals: 1
                notation: DoubleValidator.StandardNotation
            }
            onAccepted: saveButton.clicked()
        }

        DankButton {
            id: saveButton
            anchors.verticalCenter: heightField.verticalCenter
            text: "Save"
            iconName: "save"
            enabled: heightField.text.length > 0
            onClicked: {
                if (root.savePosition(editor.positionName, heightField.text))
                    heightField.text = Number(heightField.text).toFixed(1);
            }
        }
    }

    popoutWidth: 420
    popoutHeight: 510
}
