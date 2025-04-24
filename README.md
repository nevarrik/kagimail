# About

Kagi take-home project of an email client written in 40 hours over 14 days by Mark Nevarrik in Golang with tview and go-imap.

# Features

- `Compose`, `Reply`, and `Forward` emails against IMAP server
- Downloading emails from folder in background--operation be interrupted with `Esc`
- Watches for new, updated, and deleted/moved emails

# Installation

1. install golang: [Go Installation Documentation](https://go.dev/doc/install)
2. clone repo:
   ```
   git clone https://github.com/nevarrik/kagimail
   ```
3. configure email credentials in `kagimail.toml`
   ```
   cd kagimail
   vim kagimail.toml
   ```
4. run kagimail:
   ```
   go run .
   ```

# Next steps

- [ ] Use AI to pull out call to actions with deadlines
- [ ] Use AI to label the tone of each message
- [ ] Allow watching other mailboxes besides inbox by using idle commands on the hottest email folders, and by polling for later sequence numbers than the ones we have seen for cold email folders
- [ ] Replying for the use-case of writing inline comments
- [ ] Move, delete, and junk messages with undo
