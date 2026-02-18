# Improved macOS Desktop Notifications

## Problem

Current implementation in `internal/notify/darwin.go` uses bare `osascript -e 'display notification ...'`. The notification icon is always the Script Editor / Shortcuts icon because macOS attributes notifications to the process that calls the notification API, which is `/usr/bin/osascript`.

## Solution: Ship a Helper `.app` Bundle

Create a minimal `HerdNotify.app` with a Swift binary that uses `UNUserNotificationCenter`. macOS shows the **app bundle's icon** in notifications, so this gives us full branding control.

### What this gets us

- Custom herd icon in all notifications
- "Herd" appears in System Settings > Notifications (users can configure alert style, sounds, DND)
- Full `UNUserNotificationCenter` API (future: action buttons, reply fields, image attachments)
- No Homebrew dependency — ships embedded in the Go binary
- Works reliably on Sequoia
- Future-proof (official API, no private/deprecated methods)

## Implementation Plan

### 1. Swift Helper Binary (~40 lines)

The helper must use `NSApplication.run()` because `UNUserNotificationCenter.add()` is async — the process needs a RunLoop and must stay alive until the completion handler fires.

```swift
import Cocoa
import UserNotifications

class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        let center = UNUserNotificationCenter.current()
        center.delegate = self

        center.requestAuthorization(options: [.alert, .sound]) { granted, error in
            guard granted else { exit(1) }

            let content = UNMutableNotificationContent()
            content.title = CommandLine.arguments.count > 1
                ? CommandLine.arguments[1] : "Herd"
            content.body = CommandLine.arguments.count > 2
                ? CommandLine.arguments[2] : ""
            if CommandLine.arguments.count > 3 {
                content.subtitle = CommandLine.arguments[3]
            }
            content.sound = .default

            let request = UNNotificationRequest(
                identifier: UUID().uuidString,
                content: content, trigger: nil)

            center.add(request) { error in
                DispatchQueue.main.async {
                    NSApp.terminate(nil)
                }
            }
        }
    }

    // Show notification even if the helper is in foreground
    func userNotificationCenter(_ center: UNUserNotificationCenter,
                                willPresent notification: UNNotification,
                                withCompletionHandler completionHandler:
                                    @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .sound])
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.run()
```

### 2. `.app` Bundle Structure

```
HerdNotify.app/
  Contents/
    Info.plist
    MacOS/
      herd-notify       # Universal Swift binary (arm64 + x86_64)
    Resources/
      AppIcon.icns      # Custom herd icon
```

### 3. Info.plist

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key>
  <string>herd-notify</string>
  <key>CFBundleIdentifier</key>
  <string>com.herd.notify</string>
  <key>CFBundleIconFile</key>
  <string>AppIcon</string>
  <key>CFBundleName</key>
  <string>Herd</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>LSUIElement</key>
  <true/>
</dict>
</plist>
```

`LSUIElement = true` makes it a background agent app (no Dock icon, no menu bar).

### 4. Build Script

Compile a universal binary (arm64 + x86_64) at build time:

```bash
swiftc -o herd-notify-arm64 main.swift \
  -target arm64-apple-macos11 \
  -framework UserNotifications -framework Cocoa

swiftc -o herd-notify-x86 main.swift \
  -target x86_64-apple-macos11 \
  -framework UserNotifications -framework Cocoa

lipo -create -output herd-notify herd-notify-arm64 herd-notify-x86
```

No Xcode project needed — just `swiftc` from Command Line Tools.

### 5. Go Integration

**Embed the `.app` bundle** in the Go binary:

```go
//go:embed HerdNotify.app
var notifyApp embed.FS
```

**Extract on first run** to `~/.herd/HerdNotify.app/`:

- Write all files from the embedded FS to disk
- Set executable permission on the binary
- Ad-hoc sign: `exec.Command("codesign", "-s", "-", binaryPath).Run()`

**Send notifications** by exec'ing the helper:

```go
exec.Command(
    filepath.Join(herdDir, "HerdNotify.app", "Contents", "MacOS", "herd-notify"),
    title, body, subtitle,
).Run()
```

### 6. Update `darwin.go`

Replace the `osascript` call with the helper app exec. Keep `afplay` for custom sounds (or let `UNUserNotificationCenter` handle sound via `.default`). Add first-run extraction and signing logic.

## Key Constraints and Gotchas

### Signing

- **Apple Silicon requires at minimum ad-hoc signing** (`codesign -s -`) or the kernel kills the binary (`Killed: 9`). No Apple Developer account needed.
- No notarization needed — files created locally don't get the quarantine xattr, so Gatekeeper is never invoked.
- Must sign AFTER writing all files and setting permissions, BEFORE first execution.
- Do NOT use `codesign --deep` (deprecated and broken). Just sign the binary.

### First-Run Permission Prompt

- First invocation triggers macOS system dialog: "Herd Would Like to Send You Notifications"
- The helper process MUST stay alive until the user responds (clicks Allow/Deny). If it exits early, the prompt vanishes and permission is silently denied.
- After first grant, the decision is remembered forever (keyed by `CFBundleIdentifier`).
- Alternative: use `.provisional` authorization to skip the prompt and deliver "quiet" notifications (appear in Notification Center but don't interrupt). Can request full authorization later.

### Bundle Identifier is Permanent

- macOS keys notification permissions to `CFBundleIdentifier`
- If you change it, users must re-grant permission
- Pick a good one and never change it: `com.herd.notify`

### Notification Rate Limiting

- macOS throttles notifications if you send too many too fast
- No official docs on the exact limit, but >1/second per bundle causes silent drops
- Not a concern for herd's current use (status change notifications are infrequent)

### Icon File

- Need an `.icns` file for `CFBundleIconFile`
- Can create from a PNG using `iconutil` or `sips`
- Without it, notifications show the generic macOS app icon (still better than Script Editor)

## Alternatives Considered and Rejected

| Approach | Why rejected |
|---|---|
| **osascript with subtitle** | Still shows Script Editor icon, no branding |
| **terminal-notifier** | Adds Homebrew dependency; `-appIcon` uses private API that can break |
| **terminal-notifier custom build** | Complex fork/rebuild process, still uses deprecated `NSUserNotification` |
| **alerter** | Homebrew dependency; overkill for simple banners; same private API for icons |
| **Go libraries (gosx-notifier, beeep)** | All wrap terminal-notifier or osascript; same icon limitations |
| **osascript from inside .app** | Notification is still attributed to Script Editor, not the .app |
| **Compile Swift on user's machine** | Requires Command Line Tools; slow; version/arch issues |
| **NSUserNotificationCenter** | Deprecated since macOS 11; could be removed any release |

## Files to Modify

- `internal/notify/darwin.go` — Replace osascript with helper app exec, add extraction/signing
- `internal/notify/notify.go` — Possibly add an `Init()` method for first-run setup
- New: `notify-helper/main.swift` — Swift source for the helper
- New: `notify-helper/Info.plist` — App bundle plist
- New: `notify-helper/AppIcon.icns` — Herd icon
- `Makefile` — Add Swift compile + .app packaging step
