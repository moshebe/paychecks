package main

import (
	"errors"
	"io"
	"log"
	"net/textproto"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/caarlos0/env/v6"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-imap/responses"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

type paychecks struct {
	Addr         string   `env:"IMAP_ADDR" envDefault:"imap.gmail.com:993"`
	Email        string   `env:"EMAIL"`
	Password     string   `env:"PASSWORD"`
	PDFPasswords []string `env:"PDF_PASSWORDS"`
	ID           string   `env:"ID"`
	OutputDir    string   `env:"OUTPUT_DIR" envDefault:"/tmp/output"`
	Filter       filter

	client *client.Client
}

type filter struct {
	Inbox    string   `env:"FILTER_INBOX" envDefault:"Inbox"`
	Subject  string   `env:"FILTER_SUBJECT"`
	From     string   `env:"FILTER_FROM"`
	Body     []string `env:"FILTER_BODY" envSeparator:";" envDefault:"תלוש משכורת"`
	GmailRaw string   `env:"FILTER_GMAIL_RAW"`
}

var filePattern = regexp.MustCompile(`^(\d{9}_20[1-2]\d_[0-1][0-9]|\d{9}_T\d{3}|T?\d{3})\.pdf$`)

func allowedContentType(contentType string) bool {
	for _, c := range []string{"application/pdf", "application/octet-stream"} {
		if c == contentType {
			return true
		}
	}
	return false
}

func (p *paychecks) save(filename, year string, reader io.Reader) (string, error) {
	var outputPath string
	if parts := strings.Split(filename, "_"); len(parts) == 3 {
		outputPath = path.Join(p.OutputDir, parts[0], parts[1], parts[2])
	} else {
		name := strings.TrimPrefix(filename, p.ID+"_")
		if year != "" {
			name = year + "_" + name
		}
		outputPath = path.Join(p.OutputDir, p.ID, "forms", name)
	}

	if err := os.MkdirAll(path.Dir(outputPath), 0755); err != nil {
		return "", err
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		return "", err
	}

	return file.Name(), nil
}

func (p *paychecks) processMessage(r io.Reader, year string, attachment func(filename, year string, reader io.Reader) error) error {
	mr, err := mail.CreateReader(r)
	if err != nil {
		return err
	}

	if d, err := mr.Header.Date(); err == nil && !d.IsZero() {
		year = strconv.Itoa(d.Year())
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			if errors.As(err, &message.UnknownCharsetError{}) {
				continue
			}
			return err
		}

		h, ok := part.Header.(*mail.AttachmentHeader)
		if !ok {
			continue
		}

		contentType, _, err := h.ContentType()
		if err != nil {
			return err
		}

		if contentType == "message/rfc822" {
			if err := p.processMessage(part.Body, year, attachment); err != nil {
				return err
			}
			continue
		}

		if !allowedContentType(contentType) {
			continue
		}

		filename, err := h.Filename()
		if err != nil {
			return err
		}

		if !filePattern.MatchString(filename) {
			log.Printf("file %q not match the filename pattern\n", filename)
			continue
		}

		log.Printf("handle attachment: %s\n", filename)
		if err = attachment(filename, year, part.Body); err != nil {
			return err
		}
	}

	return nil
}

func (p *paychecks) fetch(results []uint32, attachment func(filename, year string, reader io.Reader) error) error {
	seqSet := new(imap.SeqSet)
	seqSet.AddNum(results...)
	messages := make(chan *imap.Message, 100)
	section := &imap.BodySectionName{}
	_ = p.client.Fetch(seqSet, []imap.FetchItem{section.FetchItem(), imap.FetchEnvelope}, messages)

	for msg := range messages {
		log.Println("* " + msg.Envelope.Subject)
		if err := p.processMessage(msg.GetBody(section), "", attachment); err != nil {
			return err
		}
	}

	return nil
}

func gmailRawSearch(c *client.Client, query string) ([]uint32, error) {
	cmd := &imap.Command{
		Name: "SEARCH",
		Arguments: []interface{}{
			imap.RawString("CHARSET"),
			imap.RawString("UTF-8"),
			imap.RawString("X-GM-RAW"),
			query,
		},
	}
	res := &responses.Search{}
	status, err := c.Execute(cmd, res)
	if err != nil {
		return nil, err
	}
	if err := status.Err(); err != nil {
		return nil, err
	}
	return res.Ids, nil
}

func New() (*paychecks, error) {
	p := &paychecks{}
	if err := env.Parse(p); err != nil {
		return nil, err
	}
	return p, nil
}

func run() error {
	log.Println("Connecting to server...")

	p, err := New()
	if err != nil {
		return err
	}

	c, err := client.DialTLS(p.Addr, nil)
	if err != nil {
		return err
	}
	p.client = c
	log.Println("Connected")

	defer func() {
		_ = c.Logout()
	}()

	if err = c.Login(p.Email, p.Password); err != nil {
		return err
	}
	log.Println("Logged in")

	_, err = c.Select(p.Filter.Inbox, true)
	if err != nil {
		return err
	}
	var searchResults []uint32
	if p.Filter.GmailRaw != "" {
		searchResults, err = gmailRawSearch(c, p.Filter.GmailRaw)
	} else {
		sc := &imap.SearchCriteria{Body: p.Filter.Body, Header: textproto.MIMEHeader{}}
		if p.Filter.From != "" {
			sc.Header.Set("From", p.Filter.From)
		}
		if p.Filter.Subject != "" {
			sc.Header.Set("Subject", p.Filter.Subject)
		}
		searchResults, err = c.Search(sc)
	}
	if err != nil {
		return err
	}
	log.Println("Search results: ", searchResults)

	err = p.fetch(searchResults, func(filename, year string, reader io.Reader) error {
		log.Printf("Found file %q", filename)
		savedFilename, err := p.save(filename, year, reader)
		if err != nil {
			return err
		}
		if err = decrypt(savedFilename, p.PDFPasswords); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.Println("Done!")

	return nil
}

func main() {
	err := run()
	if err != nil {
		log.Fatal("Failed to run: ", err)
	}
}
