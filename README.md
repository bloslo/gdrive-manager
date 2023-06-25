# gdrive-manager

This tool lists files and folders, downloads and uploads files

## Usage

Clone the repo or download it as a zip.
To build an executable run:

```
go build gdrive-manager.go
```

[Create a Google Cloud Project](https://developers.google.com/workspaceguides/create-project)

[Create OAuth client credentials for a Desktop App](https://developers.google.com/workspace/guides/create-credentials#oauth-client-id)

Download the credentials as JSON and save them in the same folder as the
executable.

The available subcommands are:

- `list`
- `download`
- `upload`

### List

Prints files/folders and their id.

The `list` subcommand has the following flags:

- `files` - List only files.
- `folders` - List only folders.
- `all` - List files and folders.

**Note!** You must pass one of the above flags!

List only files:

```
./gdrive-manager list -files
```

List only folders:

```
./gdrive-manager list -fodlers
```

List files and folders:

```
./grdrive-manager list -all
```

### Download

**Note!** Currently, downloading a folder is not supported.

The `download` subcommand has the following flags:

- `fileId` - The id of the file to be downloaded.
- `filename` - The name of the locally created file.

**Note!** Both flags are required!

The file will be downloaded in the same directory as the tool.

```
./grdrive-manager download -fileId <file-id> -filename <filename>
```

### Upload

The `upload` subcommand has the following flags:

- `filepath` - The path to the file to be uploaded. The path can be
relative or absolute

**Note!** The above flag is requred!

```
./gdrive-manager upload -filepath <path-to-file>
```
