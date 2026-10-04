# Record from the repository root with Charm VHS v0.11.0.
# Real Kafka is required. Set TRIAL_RECOVERY_EXISTING_BROKER=1 and
# TRIAL_BROKER=host:port to use an existing broker without Docker Compose.
Output docs/the-recovery.gif
Require bash
Require go

Set Shell bash
Set Theme "TokyoNight"
Set FontFamily "monospace"
Set FontSize 20
Set LineHeight 1.1
Set Width 1120
Set Height 820
Set Padding 24
Set WindowBar Colorful
Set Framerate 12
Set PlaybackSpeed 1
Set CursorBlink false
Set TypingSpeed 30ms
Set WaitTimeout 5m

Hide
Wait+Line /^>\s*$/
Sleep 1s
Type@30ms "  go build -o trial ./cmd/trial && printf '\nBUILD_READY\n'"
Enter
Wait+Screen /(?m)^BUILD_READY\s*$/
Wait+Line /^>\s*$/
Type@10ms "clear"
Enter
Sleep 200ms
Show

Type "TRIAL_RECOVERY_DELAY=1 bash docs/the-recovery-demo.sh"
Sleep 400ms
Enter
Wait+Screen /Recovery demo complete\./
Sleep 5s
