# Record from the repository root with Charm VHS v0.11.0.
# Verified stored-frame playback; the demo runs the actual deposition.
Output docs/the-orrery.gif
Require bash
Require go

Set Shell bash
Set Theme "TokyoNight"
Set FontFamily "monospace"
Set FontSize 16
Set LineHeight 1.1
Set Width 1000
Set Height 880
Set Padding 24
Set WindowBar Colorful
Set Framerate 12
Set LoopOffset 35%
Set CursorBlink false
Set TypingSpeed 30ms
Set WaitTimeout 3m

Hide
Wait+Line /^>\s*$/
Sleep 1s
# Leading spaces absorb an initial key lost during Windows PTY resize.
Type@30ms "  go build -o trial ./cmd/trial && printf '\nBUILD_READY\n'"
Enter
Wait+Screen /(?m)^BUILD_READY\s*$/
Wait+Line /^>\s*$/
Type@10ms "clear"
Enter
Sleep 200ms
Show

Type "bash docs/the-orrery-demo.sh"
Sleep 400ms
Enter
Wait+Screen /Demo complete\./
Sleep 3s
