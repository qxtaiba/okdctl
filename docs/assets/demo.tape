# README demo: hero-hub to deploy-stream walkthrough for a 3 control-plane /
# 3 worker cluster. Launches on the hub, picks "edit config", walks all ten
# configure steps (gated on the header's phase trail — connect/cluster/
# extras/review, not step numbers; see scripts/screenshot/wizard.tape.in),
# confirms the review's default "deploy now" action, then watches the
# deploy stream run its five phases against OKDCTL_WIZARD_DEMO's scripted
# feed through to the done box.
# Regenerate with scripts/demo/record.sh (or `make demo`), which builds
# the binary, starts the fake Proxmox API, seeds a saved okdctl.yaml so the
# hub renders its five-verb menu, and exports OKDCTL_DEMO_BIN /
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
Wait+Screen@10s /cluster status/

# the hub — deploy is the first verb and deploys the saved configuration
# outright; the walkthrough wants the configure flow, which is the second.
Sleep 1.5s
Down
Sleep 900ms
Enter
Wait+Screen@10s /connect ✓●○ · cluster · extras · review/

# okd version — expand recommended group, browse, confirm
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
Wait+Screen@10s /connect ✓✓● · cluster · extras · review/

# proxmox connection — initial focus lands past the host field, so back up
# to it before typing every box
Shift+Tab
Sleep 450ms
Shift+Tab
Sleep 450ms
Type "127.0.0.1:8006"
Wait+Screen@2s /127.0.0.1:8006/
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
Wait+Screen@10s /connect · cluster ●○○○ · extras · review/

# basics — name, domain, 3 control-plane / 3 workers
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
Wait+Screen@10s /connect · cluster ✓●○○ · extras · review/

# node placement — live discovery, walk every assignment
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
Wait+Screen@10s /connect · cluster ✓✓●○ · extras · review/

# networking — type every box; api vip stays blank (auto-derived)
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
Wait+Screen@10s /connect · cluster ✓✓✓● · extras · review/

# resources — type control plane and worker sizing
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
Wait+Screen@10s /connect · cluster · extras ●○○ · review/

# addons — browse the catalog
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
Wait+Screen@10s /connect · cluster · extras ✓●○ · review/

# files — type pull secret, ssh key, web root
Sleep 1.5s
Type "pull-secret.json"
Sleep 1.3s
Tab
Type ".ssh/id_ed25519.pub"
Sleep 1.4s
Tab
Type "/var/www/html"
Sleep 2s
Enter
Wait+Screen@10s /connect · cluster · extras ✓✓● · review/

# advanced — type vm ids and timeouts
Sleep 2s
Type "6000"
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
Wait+Screen@10s /connect · cluster · extras · review/

# review — scroll the whole summary, then confirm the default "deploy now"
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
Sleep 1.2s
Up
Sleep 1.4s
Up
Sleep 4s
Enter

# deploy stream — the phase checklist and log tail run against the scripted
# feed, then hand off to the done box.
Wait+Screen@10s /deploying homelab/
Sleep 3s
Wait+Screen@15s /ACCESS/
Sleep 3s
