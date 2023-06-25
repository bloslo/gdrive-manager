package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// Retrieve a token, saves the token, then returns the generate client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first time.
	tokenFile := "token.json" // TODO: Use a flag for the filename.
	token, err := tokenFromFile(tokenFile)
	if err != nil {
		log.Println("Request token from server.")
		token = getTokenFromWeb(config)
		saveToken(tokenFile, token)
	}
	return config.Client(context.Background(), token)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	ch := make(chan string)

	// Create oauthState cookie
	oauthState := generateStateOauthCookie()
	log.Printf("Oauth state: %s", oauthState)
	startLocalServer(ch, oauthState)
	log.Println("Send oauth state to channel")
	authURL := config.AuthCodeURL(oauthState, oauth2.AccessTypeOffline)
	go openURL(authURL)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	code := <-ch
	log.Printf("Got code: %s", code)

	token, err := config.Exchange(context.TODO(), code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return token
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	return token, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func generateStateOauthCookie() string {
	b := make([]byte, 16)
	rand.Read(b)
	state := base64.URLEncoding.EncodeToString(b)

	return state
}

func openURL(url string) {
	try := []string{"xdg-open"}
	for _, bin := range try {
		err := exec.Command(bin, url).Run()
		if err == nil {
			return
		}
	}
	log.Printf("Error opening URL in browser.")
}

func oauthGoogleCallback(ch chan string, oauthState string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("state") != oauthState {
			log.Println("Invalid oauth state")
			http.Error(w, "Invalid oauth state", 500)
			return
		}

		if code := r.FormValue("code"); code != "" {
			log.Printf("Received code: %s", code)
			fmt.Fprintf(w, "<h1>Successful authentication!</h1>")
			w.(http.Flusher).Flush()
			ch <- code
			return
		}

		log.Printf("No code has been received!")
		http.Error(w, "No code has been received!", 500)
	}
}

func googleAuthHandler(ch chan string, oauthState string) http.Handler {
	mux := http.NewServeMux()
	oauthGoogle := oauthGoogleCallback(ch, oauthState)
	mux.HandleFunc("/auth/google/callback", oauthGoogle)

	return mux
}

func startLocalServer(ch chan string, oauthState string) {
	server := &http.Server{
		Addr:    ":8000",
		Handler: googleAuthHandler(ch, oauthState),
	}

	log.Printf("Starting HTTP server. Listening at %q", server.Addr)
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("%v", err)
		} else {
			log.Println("Server closed!")
		}
	}()
	log.Printf("Started HTTP server.")
}

type CommandFlag struct {
}

func (cf CommandFlag) isFlagPassed(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func (cf CommandFlag) isStringFlagSet(fs *flag.FlagSet, name string) bool {
	isSet := false
	fs.Visit(func(f *flag.Flag) {
		if len(f.Value.String()) > 0 {
			isSet = true
		}
	})
	return isSet
}

func NewListCommand() *ListCommand {
	lc := &ListCommand{
		fs: flag.NewFlagSet("list", flag.ContinueOnError),
	}

	lc.fs.BoolVar(&lc.files, "files", false, "List only files")
	lc.fs.BoolVar(&lc.folders, "folders", false, "List only folders")
	lc.fs.BoolVar(&lc.all, "all", false, "List files and folders")

	return lc
}

// TODO: Move the FlagSet to the CommandFlag. See https://stackoverflow.com/a/34644202
type ListCommand struct {
	fs *flag.FlagSet
	CommandFlag

	service *drive.Service
	files   bool
	folders bool
	all     bool
}

func (l *ListCommand) Name() string {
	return l.fs.Name()
}

func (l *ListCommand) Init(args []string, service *drive.Service) error {
	l.service = service
	return l.fs.Parse(args)
}

func (l *ListCommand) Run() error {
	var err error
	var files []*drive.File

	if l.isFlagPassed(l.fs, "files") {

		// TODO: Use a flag to pass the page size.
		files, err = l.GetFileList("mimeType != 'application/vnd.google-apps.folder'", 10)

		fmt.Println("Files:")
	} else if l.isFlagPassed(l.fs, "folders") {
		fmt.Println("List folders")
		files, err = l.GetFileList("mimeType = 'application/vnd.google-apps.folder'", 10)

		fmt.Println("Folders:")
	} else if l.isFlagPassed(l.fs, "all") {
		files, err = l.GetFileList("", 10)

		fmt.Println("Files and folders:")
	} else {
		return errors.New("you must pass a flag")
	}

	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Println("No files or folders found.")
	} else {
		for _, i := range files {
			fmt.Printf("%s (%s)\n", i.Name, i.Id)
		}
	}
	return nil
}

func (l *ListCommand) GetFileList(query string, pageSize int64) ([]*drive.File, error) {
	var resp *drive.FileList
	var err error
	var files []*drive.File

	for {
		var pageToken string
		if resp != nil {
			if resp.NextPageToken == "" {
				break
			} else {
				pageToken = resp.NextPageToken
			}
		}

		resp, err = l.service.Files.List().PageSize(pageSize).
			Q(query). // Exclude folders
			Fields("nextPageToken, files(id, name)").
			PageToken(pageToken).Do()

		if err != nil {
			return nil, err
		}

		files = append(files, resp.Files...)
		fmt.Printf("Number of files: %d\n", len(files))
	}

	return files, nil
}

type DownloadCommand struct {
	fs *flag.FlagSet
	CommandFlag

	service  *drive.Service
	fileId   string
	filename string
}

func NewDownloadCommand() *DownloadCommand {
	dc := &DownloadCommand{
		fs: flag.NewFlagSet("download", flag.ContinueOnError),
	}

	dc.fs.StringVar(&dc.fileId, "fileId", "", "The id of the file to be downloaded")
	dc.fs.StringVar(&dc.filename, "filename", "", "Name of the locally created file")

	return dc
}

func (d *DownloadCommand) Name() string {
	return d.fs.Name()
}

func (d *DownloadCommand) Init(args []string, service *drive.Service) error {
	d.service = service
	return d.fs.Parse(args)
}

func (d *DownloadCommand) Run() error {
	if !(d.isStringFlagSet(d.fs, "fileId") && d.isStringFlagSet(d.fs, "filename")) {
		return errors.New("you must set the value of the fileId and filename flags")
	}

	respFile, err := d.service.Files.Get(d.fileId).Download()
	if err != nil {
		return err
	}

	contentType := respFile.Header.Get("Content-Type")
	extensions, err := mime.ExtensionsByType(contentType)
	if err != nil {
		return err
	}
	fmt.Printf("Extensions: %v", extensions)

	bytes, err := io.ReadAll(respFile.Body)
	if err != nil {
		return err
	}
	respFile.Body.Close()

	return os.WriteFile(fmt.Sprintf("%s%s", d.filename, extensions[len(extensions)-1]), bytes, 0644)
}

type UploadCommand struct {
	fs *flag.FlagSet
	CommandFlag

	service  *drive.Service
	filepath string
}

func NewUploadCommand() *UploadCommand {
	uplC := &UploadCommand{
		fs: flag.NewFlagSet("upload", flag.ContinueOnError),
	}

	uplC.fs.StringVar(&uplC.filepath, "filepath", "", "File path of the file to be uploaded")

	return uplC
}

func (upl *UploadCommand) Name() string {
	return upl.fs.Name()
}

func (upl *UploadCommand) Init(args []string, service *drive.Service) error {
	upl.service = service
	return upl.fs.Parse(args)
}

func (upl *UploadCommand) Run() error {
	if !upl.isStringFlagSet(upl.fs, "filepath") {
		return errors.New("you must set the value of the filepath flag")
	}

	// Check if filename has extension.
	absPath, err := filepath.Abs(upl.filepath)
	if err != nil {
		return err
	}

	ext := filepath.Ext(upl.filepath)
	if ext == "" {
		return errors.New("filename must have an extension")
	}

	driveFile := &drive.File{
		Name:     filepath.Base(upl.filepath),
		MimeType: mime.TypeByExtension(ext),
	}

	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer file.Close()

	resp, err := upl.service.Files.
		Create(driveFile).
		Media(file).
		ProgressUpdater(func(now, size int64) { fmt.Printf("%d, %d\r", now, size) }).
		Do()

	if err != nil {
		return err
	}

	fmt.Printf("File Id: %s\n", resp.Id)
	return nil
}

type Runner interface {
	Init([]string, *drive.Service) error
	Run() error
	Name() string
}

func root(args []string, service *drive.Service) error {
	if len(args) < 1 {
		return errors.New("you must pass a sub-command")
	}

	cmds := []Runner{
		NewListCommand(),
		NewDownloadCommand(),
		NewUploadCommand(),
	}

	subcommand := os.Args[1]

	for _, cmd := range cmds {
		if cmd.Name() == subcommand {
			fmt.Println(os.Args[2:])
			err := cmd.Init(os.Args[2:], service)
			if err != nil {
				return fmt.Errorf("failed to parse arguments for subcommand: %s", subcommand)
			}
			return cmd.Run()
		}
	}

	return fmt.Errorf("unknown subcommand: %s", subcommand)
}

func main() {
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json") // TODO: Use a flag for the credentials filename.
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	service, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Drive client: %v", err)
	}

	if err := root(os.Args[1:], service); err != nil {
		log.Fatalf("Subcommand %s failed: %v", os.Args[0], err)
		os.Exit(1)
	}
}
