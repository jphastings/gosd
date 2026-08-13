# Settings for this device

This folder holds the settings for this device. Each setting is its own
plain text file — there is one file per setting, and the file's name
tells you what it controls.

To change a setting, open its file in a plain text editor — for example
Notepad (Windows), TextEdit (Mac, but see the note below), or nano
(Linux) — type the value you want, and save the file. Put the card back
in the device and it will use the new value the next time it starts.

IMPORTANT if you use TextEdit on a Mac: click Format > Make Plain Text
before you save, or the file will stop working.

A file that is empty (nothing typed in it at all) means that setting is
simply not set. That is a normal, valid state — many settings are meant
to be left empty until you need them.

Next to most setting files you will find a matching file ending in
`.explain.md`, like this one. Those files are documentation only:
changing them does nothing to the device. Read them to understand what
a setting does before you change it.

Folders group related settings together, and each folder has its own
`explain.md` describing what the whole group is for.

After the device's software is updated, you may occasionally see extra
files appear next to a setting:

- `<name>.new` shows the value the device's new software would use for
  that setting by default. Your own setting is unaffected and stays in
  force — this file is just letting you know a new default exists.
- `<name>.unused` holds a setting you previously configured that no
  longer applies to this version of the software. It is kept here so
  your value isn't lost, in case you need it again.

Neither of these needs any action from you.
