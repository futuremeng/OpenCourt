// ocr_helper: macOS 系统 Vision OCR 极小 CLI（plan-v0.4 O1/D1）。
// 用法: ocr_helper <图片路径>
// 输出: stdout 一行 JSON {"text": "...", "confidence": 0.0-1.0}
// 纪律: 只读指定文件、零网络、零模型自动改字；置信度 = 各行最优候选均值。
// 构建: scripts/build-ocr-helper.sh（swiftc，无需 Xcode 工程）。
import AppKit
import Foundation
import Vision

let args = CommandLine.arguments
guard args.count == 2 else {
    FileHandle.standardError.write("usage: ocr_helper <image>\n".data(using: .utf8)!)
    exit(2)
}
guard let image = NSImage(contentsOfFile: args[1]),
      let cg = image.cgImage(forProposedRect: nil, context: nil, hints: nil) else {
    print("{\"error\": \"image unreadable\"}")
    exit(3)
}
let request = VNRecognizeTextRequest()
request.recognitionLanguages = ["zh-Hans", "zh-Hant", "en-US"]
request.recognitionLevel = .accurate
request.usesLanguageCorrection = true
let handler = VNImageRequestHandler(cgImage: cg, options: [:])
do {
    try handler.perform([request])
} catch {
    print("{\"error\": \"vision failed\"}")
    exit(4)
}
let observations = request.results ?? []
var lines: [String] = []
var sum = 0.0
for o in observations {
    guard let candidate = o.topCandidates(1).first else { continue }
    lines.append(candidate.string)
    sum += Double(candidate.confidence)
}
let confidence = observations.isEmpty ? 0.0 : sum / Double(observations.count)
let payload: [String: Any] = ["text": lines.joined(separator: "\n"), "confidence": confidence]
guard let data = try? JSONSerialization.data(withJSONObject: payload) else { exit(5) }
FileHandle.standardOutput.write(data)