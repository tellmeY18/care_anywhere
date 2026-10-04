import AppKit
import Foundation

let directory = URL(fileURLWithPath: CommandLine.arguments[1])
try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
for size in [16, 32, 128, 256, 512] {
    for scale in [1, 2] {
        let pixels = size * scale
        let image = NSImage(size: NSSize(width: pixels, height: pixels))
        image.lockFocus()
        let p = CGFloat(pixels)
        NSColor(calibratedRed: 0.016, green: 0.471, blue: 0.341, alpha: 1).setFill()
        NSBezierPath(roundedRect: NSRect(x: p * 0.06, y: p * 0.06, width: p * 0.88, height: p * 0.88), xRadius: p * 0.2, yRadius: p * 0.2).fill()
        NSColor.white.setFill()
        NSBezierPath(roundedRect: NSRect(x: p * 0.24, y: p * 0.42, width: p * 0.52, height: p * 0.16), xRadius: p * 0.035, yRadius: p * 0.035).fill()
        NSBezierPath(roundedRect: NSRect(x: p * 0.42, y: p * 0.24, width: p * 0.16, height: p * 0.52), xRadius: p * 0.035, yRadius: p * 0.035).fill()
        image.unlockFocus()
        let bitmap = NSBitmapImageRep(data: image.tiffRepresentation!)!
        let suffix = scale == 2 ? "@2x" : ""
        try bitmap.representation(using: .png, properties: [:])!.write(to: directory.appendingPathComponent("icon_\(size)x\(size)\(suffix).png"))
    }
}
