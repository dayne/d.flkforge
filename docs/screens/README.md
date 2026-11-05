# Wizard screens (PLAN M4)

Captured 2026-11-05 with `tmux capture-pane` from the real binary in a
real terminal, at 80x24 and 160x48, following PLAN M4's acceptance flow:
pick the steam-game template, name it, fill in the params it leaves open
(the Steam app id, the server command, the port, the ready line), swap
backup-pause for backup-stop on the Backups step, review, write.

    01-start            the templates
    02-name             the app name
    03-software         typing the Steam app id; the preview marks the lines it changes (▌)
    04-process          the server command (a list)
    05-network          the game port
    06-health           the ready line for the log check
    07-backups-picker   "+ add a backup snippet…": the picker
    08-backups-swapped  backup-stop replaced backup-pause, keeping the params they share
    09-help             F1
    10-review           the result, flk config show's verdict, the Write button
    11-written          the four-part report

Colours and the reversed cursor don't show in these captures (they're
plain text); the golden snapshots in internal/wizard/testdata are the
same screens from the tests.
