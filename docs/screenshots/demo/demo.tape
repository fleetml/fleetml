# Terminal part of docs/screenshots/demo.gif; record.sh runs it from the
# repository root with DEMO_INFO set and -o for the output. The setup
# (builds, server, agents) is not recorded.
Set Shell bash
Set FontSize 15
Set Width 1280
Set Height 760
Set Padding 24
Set Theme "GitHub Dark"
Set TypingSpeed 0

Hide
Type "WAIT_FOR_ENTER=1 ./scripts/demo-agent.sh"
Enter
Wait+Screen@4m /press Enter to start the story/
Enter
Sleep 300ms
Show
Wait+Screen@2m /Done in/
Sleep 4s
Hide
# record.sh takes the dashboard screenshots now; the demo exits when they are done.
Wait@3m
