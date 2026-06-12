Output shale-init.gif

Set FontSize 15
Set Width 960
Set Height 600
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 55ms

Hide
Type `cd $(mktemp -d) && git init -q -b main && git config user.email dev@team.com && git config user.name Dev && PS1='your-repo $ ' && clear`
Enter
Sleep 500ms
Show

Sleep 1s
Type "shale init"
Sleep 500ms
Enter
Sleep 3s
Type "y"
Sleep 1s
Enter
Sleep 5s
Hide
Type "exit"
Enter
