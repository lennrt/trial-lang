# Record from the repository root with Charm VHS v0.11.0.
# Frames come from a fresh, complete deposition; the runner only styles playback.
Output docs/the-castle.gif
Require bash
Require go

Set Shell bash
Set Theme "TokyoNight"
Set FontFamily "monospace"
Set FontSize 20
Set LineHeight 1.1
Set Width 1200
Set Height 660
Set Padding 24
Set WindowBar Colorful
Set Framerate 12
Set LoopOffset 50%
Set CursorBlink false
Set TypingSpeed 30ms
Set WaitTimeout 6m

Hide
Wait+Line /^>\s*$/
Sleep 1s
# Leading spaces also absorb an initial key lost during Windows PTY resize.
Type@30ms "  go build -o trial ./cmd/trial && printf '\nBUILD_READY\n'"
Enter
Wait+Screen /(?m)^BUILD_READY\s*$/
Wait+Line /^>\s*$/
Type@10ms "clear"
Enter
Sleep 200ms
Show

Type "bash docs/the-castle-demo.sh"
Sleep 400ms
Enter
Wait+Screen /Demo complete\./
Sleep 4s
