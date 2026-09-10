import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui

Panel {
  id: root
  moduleName: "edofic.idasen-desk"
  ipcTarget: "edofic.idasen-desk"

  property real deskHeight: 0
  property real deskSpeed: 0
  property string controllerStatus: "connecting"
  property string errorMessage: ""
  property var positions: ({})
  property bool shuttingDown: false
  readonly property string configuredBinaryPath: String(setting("binaryPath", ""))
  readonly property string binaryPath: configuredBinaryPath || (Quickshell.env("HOME") || "") + "/.local/bin/idasen"
  readonly property int adjustmentMm: Math.max(1, Number(setting("adjustmentMm", 5)) || 5)
  readonly property bool connected: deskHeight > 0 && controllerStatus !== "unavailable" && controllerStatus !== "disconnected"
  readonly property string heightText: connected ? (deskHeight * 100).toFixed(1) + " cm" : "--.- cm"

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  function send(command) {
    if (!controllerProcess.running) { showError("Desk controller is not running"); return }
    controllerProcess.write(JSON.stringify(command) + "\n")
  }
  function showError(message) { errorMessage = message; errorTimer.restart() }
  function moveToPosition(name) { send({"action": "move", "position": name}) }
  function adjust(direction) { send({"action": "adjust", "delta": direction * adjustmentMm / 1000.0}) }
  function positionText(name) {
    var value = positions[name]
    return value === undefined ? "" : (value * 100).toFixed(1)
  }
  function savePosition(name, centimetres) {
    var parsed = Number(centimetres)
    if (!Number.isFinite(parsed) || parsed < 62 || parsed > 127) {
      showError("Height must be between 62 and 127 cm")
      return false
    }
    send({"action": "set_position", "position": name, "height": parsed / 100.0})
    return true
  }
  function handleEvent(line) {
    if (!String(line).trim()) return
    try {
      var event = JSON.parse(line)
      if (event.status) controllerStatus = event.status
      if (event.type === "height") {
        deskHeight = event.height
        deskSpeed = event.speed || 0
      } else if (event.type === "positions") {
        positions = event.positions || {}
      } else if (event.type === "status" && event.positions) {
        positions = event.positions
      } else if (event.type === "error") {
        showError(event.message || "Unknown desk error")
      }
    } catch (error) {
      console.warn("IDÅSEN: invalid controller event:", line, error)
    }
  }

  Process {
    id: controllerProcess
    command: [
      "bash",
      "-c",
      "test -f \"${XDG_CONFIG_HOME:-$HOME/.config}/idasen/idasen.yaml\" || exit 78; exec \"$1\" controller",
      "idasen-widget",
      root.binaryPath
    ]
    stdinEnabled: true
    running: false
    stdout: SplitParser { onRead: function(line) { root.handleEvent(line) } }
    stderr: SplitParser { onRead: function(line) { if (String(line).trim()) console.warn("IDÅSEN:", line) } }
    onExited: function(exitCode) {
      root.controllerStatus = "disconnected"
      if (!root.shuttingDown) {
        if (exitCode === 78) {
          root.errorMessage = "Run idasen init to configure the desk"
        } else {
          if (!root.errorMessage) root.showError("Desk controller exited (code " + exitCode + ")")
          restartTimer.restart()
        }
      }
    }
  }
  Timer {
    id: restartTimer
    interval: 5000
    onTriggered: { root.controllerStatus = "connecting"; controllerProcess.running = true }
  }
  Timer { id: errorTimer; interval: 6000; onTriggered: root.errorMessage = "" }
  Component.onCompleted: controllerProcess.running = true
  Component.onDestruction: { shuttingDown = true; controllerProcess.running = false }

  WidgetButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: root.heightText
    fontSize: Style.font.caption
    horizontalMargin: 7
    verticalPadding: 7
    tooltipText: "IDÅSEN Desk"
    onPressed: root.toggle()
  }

  KeyboardPanel {
    anchorItem: button
    owner: root
    bar: root.bar
    open: root.opened
    contentWidth: fittedContentWidth(Style.space(420))
    contentHeight: fittedContentHeight(content.implicitHeight)

    Column {
      id: content
      anchors.left: parent.left
      anchors.right: parent.right
      anchors.top: parent.top
      spacing: Style.space(14)

      Text {
        width: parent.width
        text: root.heightText
        horizontalAlignment: Text.AlignHCenter
        color: root.errorMessage ? Color.urgent : root.bar.foreground
        font.family: root.bar.fontFamily
        font.pixelSize: Style.font.displayLarge
        font.bold: true
      }
      Text {
        width: parent.width
        text: root.errorMessage || (root.controllerStatus === "moving" ? "Desk is moving" : root.connected ? "Live height" : "Connecting to desk…")
        horizontalAlignment: Text.AlignHCenter
        wrapMode: Text.WordWrap
        color: root.errorMessage ? Color.urgent : root.bar.foreground
        opacity: root.errorMessage ? 1 : 0.6
        font.family: root.bar.fontFamily
        font.pixelSize: Style.font.bodySmall
      }

      Row {
        width: parent.width
        spacing: Style.space(6)
        Button {
          width: (parent.width - parent.spacing) / 2
          text: "Sit" + (root.positions.sit === undefined ? "" : " · " + Math.round(root.positions.sit * 100) + " cm")
          foreground: root.bar.foreground; fontFamily: root.bar.fontFamily; bordered: true
          enabled: root.connected && root.positions.sit !== undefined
          onClicked: root.moveToPosition("sit")
        }
        Button {
          width: (parent.width - parent.spacing) / 2
          text: "Stand" + (root.positions.stand === undefined ? "" : " · " + Math.round(root.positions.stand * 100) + " cm")
          foreground: root.bar.foreground; fontFamily: root.bar.fontFamily; bordered: true
          enabled: root.connected && root.positions.stand !== undefined
          onClicked: root.moveToPosition("stand")
        }
      }

      Row {
        width: parent.width
        spacing: Style.space(6)
        Button { width: (parent.width - parent.spacing * 2) / 3; text: "Down"; foreground: root.bar.foreground; fontFamily: root.bar.fontFamily; bordered: true; enabled: root.connected; onClicked: root.adjust(-1) }
        Button { width: (parent.width - parent.spacing * 2) / 3; text: "Stop"; foreground: root.bar.foreground; fontFamily: root.bar.fontFamily; bordered: true; enabled: root.connected; onClicked: root.send({"action": "stop"}) }
        Button { width: (parent.width - parent.spacing * 2) / 3; text: "Up"; foreground: root.bar.foreground; fontFamily: root.bar.fontFamily; bordered: true; enabled: root.connected; onClicked: root.adjust(1) }
      }

      Text {
        width: parent.width
        text: "Fine adjustment: " + root.adjustmentMm + " mm"
        horizontalAlignment: Text.AlignHCenter
        color: root.bar.foreground; opacity: 0.6
        font.family: root.bar.fontFamily; font.pixelSize: Style.font.bodySmall
      }

      PanelSeparator { foreground: root.bar.foreground }
      PositionEditor { width: parent.width; positionName: "sit"; label: "Sit height" }
      PositionEditor { width: parent.width; positionName: "stand"; label: "Stand height" }
    }
  }

  component PositionEditor: Row {
    required property string positionName
    required property string label
    spacing: Style.space(8)
    Text {
      width: Style.space(85)
      anchors.verticalCenter: parent.verticalCenter
      text: parent.label
      color: root.bar.foreground
      font.family: root.bar.fontFamily
      font.pixelSize: Style.font.bodySmall
    }
    TextField {
      id: heightField
      width: parent.width - parent.children[0].width - saveButton.width - parent.spacing * 2
      placeholderText: "62–127 cm"
      text: root.positionText(parent.positionName)
      foreground: root.bar.foreground
      font.family: root.bar.fontFamily
      validator: DoubleValidator { bottom: 62; top: 127; decimals: 1; notation: DoubleValidator.StandardNotation }
      onAccepted: saveButton.clicked()
    }
    Button {
      id: saveButton
      anchors.verticalCenter: parent.verticalCenter
      text: "Save"
      foreground: root.bar.foreground; fontFamily: root.bar.fontFamily; bordered: true
      enabled: heightField.text.length > 0
      onClicked: if (root.savePosition(parent.positionName, heightField.text)) heightField.text = Number(heightField.text).toFixed(1)
    }
  }
}
