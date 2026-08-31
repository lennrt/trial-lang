# Record from the repository root with Charm VHS v0.11.0.
Output docs/the-orrery.gif

Set Shell bash
Set Theme "TokyoNight"
Set FontFamily "JetBrains Mono"
Set FontSize 13
Set Width 1120
Set Height 720
Set Padding 20
Set Framerate 30
Set PlaybackSpeed 1
Set TypingSpeed 22ms

Hide
Type "go build -o /tmp/triallang-orrery-trial ./cmd/trial"
Enter
Sleep 1s
Type "clear"
Enter
Sleep 500ms
Show

Type "tail -n 14 examples/the-orrery.trial | head -n 4"
Enter
Sleep 3s

Hide
Type "clear"
Enter
Sleep 500ms
Show

Type "TRIAL_BIN=/tmp/triallang-orrery-trial ./docs/the-orrery-demo.sh"
Enter
Sleep 7s
