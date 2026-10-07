# Regenerate with: make demo
Output docs/demo.gif
Set Shell bash
Set FontSize 18
Set Width 1000
Set Height 560
Set TypingSpeed 60ms

# Throwaway home and fake AI commands, so real accounts are never touched.
Hide
Type `export HOME=$(mktemp -d) PS1='$ ' && mkdir -p $HOME/bin && printf '#!/bin/sh\necho Welcome to the assistant\n' > $HOME/bin/claude && cp $HOME/bin/claude $HOME/bin/codex && chmod +x $HOME/bin/* && export PATH=$PWD:$HOME/bin:$PATH && devswitch list >/dev/null && sleep 1 && clear`
Enter
Show

Type "devswitch add claude work"
Enter
Sleep 1s
Type "devswitch add claude personal"
Enter
Sleep 1s
Type "devswitch list"
Enter
Sleep 2s
Type "devswitch run claude work"
Enter
Sleep 2s
