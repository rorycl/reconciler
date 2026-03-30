## sfbatcher

This is a command-line batch processing program that is part of the
reconciler project, taking a full reconciler [configuration
file](../../config/config.example.yaml) and an Excel file.

### Usage

```
NAME:
   sfbatcher - batch update salesforce records

USAGE:
   sfbatcher [global options] [command [command options]]

DESCRIPTION:
   A batch update program for linking or unlinking salesforce donation
   records, to assist in reconciling salesforce donation records with xero
   invoice and bank transaction references.

   Please see the project README for more information at
   https://github.com/rorycl/reconciler.

   Note that this program can affect many salesforce records in one
   invocation.

COMMANDS:
   link     link salesforce records with references
   unlink   unlink salesforce records
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h  show help
```

### More info

For more information about the project, please see the main project
[README](https://github.com/rorycl/reconciler).
