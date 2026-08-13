# App settings

This folder holds extra settings used by the app running on this
device — the software this device was built to run, rather than the
device itself.

Each setting is its own file. The file's NAME is the name of the
setting, and whatever you type into the file is its value. For
example, a file in this folder named `GREETING` containing `Hello!`
sets the `GREETING` setting to `Hello!`.

Which settings exist here, and what each one does, depends entirely on
the app this device was built to run — the app's author decides what
settings it needs and should document each one with its own
`.explain.md` file alongside it. If a setting you expect isn't listed
here, check with wherever you got this device or its software.

Files whose names start with `GOSD_` are reserved for the device's own
software and are ignored by the app — don't use that prefix for your
own settings.
