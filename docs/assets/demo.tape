# VHS tape for the README demo (https://github.com/charmbracelet/vhs).
# Regenerate from the repo root with okdctl on PATH:
#   vhs docs/assets/demo.tape
# The wizard elevates via sudo before it starts; record on a host where
# sudo -v is already cached (or a NOPASSWD dev box) so no prompt appears.
Output docs/assets/demo.gif
Set FontSize 15
Set Width 1000
Set Height 640
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms
Set Shell zsh

Hide
Type `cd "$(mktemp -d)" && clear`
Enter
Show

Type "okdctl deploy"
Sleep 500ms
Enter
Sleep 4.5s
Enter
Sleep 4s
Down
Sleep 700ms
Down
Sleep 700ms
Tab
Sleep 900ms
Up
Sleep 700ms
Up
Sleep 700ms
Tab
Sleep 1.2s
Sleep 1.5s
Ctrl+C
Sleep 1.2s
