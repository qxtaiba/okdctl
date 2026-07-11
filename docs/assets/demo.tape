# README demo: full wizard walkthrough for a 3 control-plane / 3 worker
# cluster, quitting from the review screen — nothing deploys.
# Regenerate with scripts/demo/record.sh (or `make demo`), which builds
# the binary, starts the fake Proxmox API, and exports OKDCTL_DEMO_BIN /
# OKDCTL_DEMO_HOME / OKDCTL_DEMO_CWD consumed by the hidden setup line.
Output docs/assets/demo.gif
Set FontSize 18
Set Width 1500
Set Height 980
Set Padding 24
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set WindowBarSize 36
Set Margin 16
Set MarginFill "#11111b"
Set BorderRadius 10
Set TypingSpeed 110ms
Set PlaybackSpeed 2
Set Shell zsh

Hide
Type `export PATH="$(dirname "$OKDCTL_DEMO_BIN"):$PATH" HOME="$OKDCTL_DEMO_HOME" OKDCTL_WIZARD_DEMO=1; cd "$OKDCTL_DEMO_CWD"; clear`
Enter
Show

# launch
Type "okdctl deploy"
Sleep 600ms
Enter
Sleep 4.5s

# step 1/11 welcome
Enter
Sleep 8s

# step 2/11 okd version — expand recommended group, browse, confirm
Enter
Sleep 2s
Down
Sleep 1.4s
Down
Sleep 1.4s
Up
Sleep 1.4s
Up
Sleep 2s
Enter
Sleep 4.5s

# step 3/11 proxmox connection — type every box
Type "127.0.0.1:8006"
Sleep 1.4s
Tab
Type "root@pam"
Sleep 1.4s
Tab
Type "demo-password"
Sleep 1.4s
Tab
Sleep 1.2s
Tab
Sleep 1.2s
Right
Sleep 2s
Enter
Sleep 4.5s

# step 4/11 basics — name, domain, 3 control-plane / 3 workers
Type "homelab"
Sleep 1.4s
Tab
Type "lab.example.com"
Sleep 1.4s
Tab
Type "3"
Sleep 1.2s
Tab
Type "3"
Sleep 2s
Enter
Sleep 4.5s

# step 5/11 node placement — live discovery, walk every assignment
Sleep 4.5s
Tab
Sleep 1.2s
Tab
Sleep 1.2s
Tab
Sleep 1.2s
Tab
Sleep 1.2s
Tab
Sleep 1.2s
Tab
Sleep 1.2s
Tab
Sleep 1.2s
Right
Sleep 1.2s
Left
Sleep 1.2s
Tab
Sleep 1.2s
Tab
Sleep 1.2s
Tab
Sleep 2s
Enter
Sleep 4.5s

# step 6/11 networking — type every box; api vip stays blank (auto-derived)
Sleep 2s
Type "192.168.1.0/24"
Sleep 1.2s
Tab
Type "192.168.1.1"
Sleep 1.2s
Tab
Type "192.168.1.1"
Sleep 1.2s
Tab
Type "10.128.0.0/14"
Sleep 1.2s
Tab
Type "172.30.0.0/16"
Sleep 1.2s
Tab
Sleep 1.2s
Tab
Type "192.168.1.140"
Sleep 1.2s
Tab
Type "ens18"
Sleep 1.2s
Tab
Type "192.168.1.20"
Sleep 1.2s
Tab
Sleep 1.5s
Enter
Sleep 4.5s

# step 7/11 resources — type control plane and worker sizing
Sleep 2s
Type "4"
Sleep 1s
Tab
Type "12288"
Sleep 1s
Tab
Type "50"
Sleep 1s
Tab
Type "8"
Sleep 1s
Tab
Type "20480"
Sleep 1s
Tab
Type "50"
Sleep 1s
Tab
Type "500"
Sleep 1s
Tab
Type "0"
Sleep 2s
Enter
Sleep 4.5s

# step 8/11 addons — browse the catalog
Sleep 2.5s
Down
Sleep 1.3s
Down
Sleep 1.3s
Up
Sleep 1.3s
Up
Sleep 2s
Enter
Sleep 4.5s

# step 9/11 files — type pull secret, ssh key, ignition server
Sleep 1.5s
Type "~/pull-secret.json"
Sleep 1.3s
Tab
Type "~/.ssh/id_ed25519.pub"
Sleep 1.4s
Tab
Type "443"
Sleep 1.2s
Tab
Type "/var/www/html"
Sleep 2s
Enter
Sleep 4.5s

# step 10/11 advanced — type vm ids and timeouts
Sleep 2s
Type "6000"
Sleep 1.2s
Tab
Sleep 1.2s
Tab
Sleep 1.2s
Tab
Type "3600"
Sleep 1.2s
Tab
Type "7200"
Sleep 1.2s
Tab
Sleep 1.2s
Tab
Sleep 1.2s
Tab
Sleep 2s
Enter
Sleep 4.5s

# step 11/11 review — scroll the whole summary
Sleep 5s
Down
Sleep 1.2s
Down
Sleep 1.2s
Down
Sleep 1.2s
Down
Sleep 1.2s
Down
Sleep 1.2s
Down
Sleep 1.2s
Down
Sleep 1.2s
Down
Sleep 1.4s
Up
Sleep 1s
Up
Sleep 4s
Ctrl+C
Sleep 1.5s
