package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/amp-space/amp-sdk-go/crates"
	"github.com/pkg/errors"
	"xojoc.pw/useragent"
)

func main() {
	sg := NewServer()

	err := sg.ReadFiles()
	if err != nil {
		log.Fatal(err)
	}

	sg.Serve()
}

const TimeFormat = "Mon, 02 Jan 2006 15:04:05 GMT"
const DefaultOrg = "spaces.plan.tools"
const ServerDesc = "amp-space.systems/arcgate"

type OrgService struct {
	AppVars   crates.AppVars
	WellKnown map[string][]byte
	Modified  time.Time
	ModTime   string
	linkTmpl  *template.Template
}

type fileEntry struct {
	ContentType string
	ModTime     string
	FileSz      int64
	FileData    []byte
	tmpl        *template.Template
}

var CacheSizeLimit = int64(100 * 1024)

func (entry *fileEntry) recache() bool {
	return int64(len(entry.FileData)) < (entry.FileSz) && entry.FileSz < CacheSizeLimit && entry.tmpl == nil
}

type Server struct {
	homeDir   string
	server    *http.Server
	orgs      map[string]*OrgService
	statusErr int

	wwwPath string
	www     map[string]fileEntry
	wwwMu   sync.RWMutex
}

func NewServer() *Server {
	s := &Server{}

	if s.homeDir == "" {
		s.homeDir, _ = os.Getwd()
	}

	return s
}

const (
	AppVarsName      = "app-vars.arcgate.json"
	OpenLinkTmplName = "open-link.tmpl.html"
)

// ReadFiles reads each set of "org" files from the "orgs/" dir.
//
// Each org file also doubles as a file that is served from the site root ("/") and also from ("/.well-known/")
func (rs *Server) ReadFiles() error {
	rs.orgs = make(map[string]*OrgService)

	orgDir := path.Join(rs.homeDir, "orgs")
	orgDirs, err := ioutil.ReadDir(orgDir)
	if err != nil {
		return err
	}

	for _, info := range orgDirs {
		if !info.IsDir() {
			continue
		}

		orgPath := path.Join(orgDir, info.Name())
		orgFiles, err := ioutil.ReadDir(orgPath)
		if err != nil {
			return err
		}

		org := &OrgService{
			WellKnown: make(map[string][]byte),
		}

		for _, orgFile := range orgFiles {
			itemName := orgFile.Name()
			itemPath := path.Join(orgPath, itemName)

			info, _ := os.Stat(itemPath)
			if info.ModTime().After(org.Modified) {
				org.Modified = info.ModTime()
			}

			if itemName == OpenLinkTmplName {
				org.linkTmpl, err = template.ParseFiles(itemPath)
				if err != nil {
					return errors.Wrapf(err, "error parsing template %v", itemPath)
				}
				continue
			}

			itemBytes, err := ioutil.ReadFile(itemPath)
			if err != nil {
				return errors.Wrapf(err, "error reading %v", itemPath)
			}
			org.WellKnown[itemName] = itemBytes

			if itemName == AppVarsName {
				err = json.Unmarshal(itemBytes, &org.AppVars)
				if err != nil {
					return errors.Wrapf(err, "error parsing %v", itemPath)
				}
			}
		}

		if org.AppVars.AppDomain == "" {
			return errors.Errorf("%v not found for %v", AppVarsName, orgPath)
		}

		org.ModTime = org.Modified.UTC().Format(TimeFormat)

		rs.orgs[org.AppVars.AppDomain] = org
		fmt.Printf("Loaded service for %v (%v)\n", org.AppVars.AppDesc, org.AppVars.AppDomain)
	}

	err = rs.RescanWWW()
	if err != nil {
		return err
	}

	return nil
}

func (rs *Server) redirectToHTTPS(httpAddrSuffix, tlsAddrSuffix string) {
	log.Printf("Redirecting %v to %v...\n", httpAddrSuffix, tlsAddrSuffix)
	httpSrv := http.Server{
		Addr: httpAddrSuffix,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			rs.logReq(req)
			switch req.URL.Path {
			case "":
				fallthrough
			case "/":
				fallthrough
			case "/index.html":
				rs.serveFile(w, req, "/", nil)
				return
			}

			host, _, _ := net.SplitHostPort(req.Host)
			if host == "" {
				host = req.Host
			}
			reqURL := req.URL
			reqURL.Host = host + tlsAddrSuffix
			reqURL.Scheme = "https"
			http.Redirect(w, req, reqURL.String(), http.StatusMovedPermanently)
		}),
	}
	log.Println(httpSrv.ListenAndServe())
}

func (rs *Server) readSubDirs(subPath string) error {
	dirItems, err := ioutil.ReadDir(path.Join(rs.wwwPath, subPath))
	if err != nil {
		return errors.Errorf("failed to read www dir %v", subPath)
	}

	for _, item := range dirItems {
		itemPath := path.Join(subPath, item.Name())
		if item.IsDir() {
			rs.readSubDirs(itemPath)
		} else {

			entry := fileEntry{
				FileSz:      item.Size(),
				ModTime:     item.ModTime().UTC().Format(TimeFormat),
				ContentType: mime.TypeByExtension(filepath.Ext(item.Name())),
			}

			if strings.Contains(item.Name(), ".tmpl.") {
				fname := path.Join(rs.wwwPath, itemPath)
				entry.tmpl, err = template.ParseFiles(fname)
				if err != nil {
					return errors.Wrapf(err, "error parsing template %v", fname)
				}
				pubName := strings.Replace(item.Name(), ".tmpl.", ".", 1)
				itemPath = path.Join(subPath, pubName)
			}

			rs.www[itemPath] = entry
		}
	}
	return nil
}

func (rs *Server) RescanWWW() error {

	rs.wwwMu.Lock()
	defer rs.wwwMu.Unlock()

	rs.wwwPath = path.Join(rs.homeDir, "www", "")
	rs.www = make(map[string]fileEntry)

	err := rs.readSubDirs("")
	return err
}

func (rs *Server) Serve() {

	rs.server = &http.Server{
		Addr:    ":443",
		Handler: rs,
	}
	go rs.redirectToHTTPS(":80", rs.server.Addr)

	log.Printf("Listening on %v...", rs.server.Addr)
	//err := rs.server.ListenAndServe()
	err := rs.server.ListenAndServeTLS(
		path.Join(rs.homeDir, "certs/chained.wildcard.plan.tools.crt"),
		path.Join(rs.homeDir, "certs/wildcard.plan.tools.key"),
	)
	if err != nil {
		log.Fatal(err)
	}
}

var kLogPrefix = ">>> "
var kLogSpacer = []byte("   ")
var kLogEOL = []byte{'\n'}
var kLogSpaces = []byte("           ")

func (rs *Server) logReq(req *http.Request) {
	w := log.Writer()
	w.Write([]byte(kLogPrefix))
	w.Write([]byte(req.RemoteAddr))
	w.Write(kLogSpacer)
	w.Write([]byte(req.URL.String()))
	w.Write(kLogEOL)
}

func (rs *Server) logAgent(req *http.Request, appOS, agentStr string) {
	w := log.Writer()
	w.Write([]byte(kLogPrefix))
	w.Write([]byte(req.RemoteAddr))
	w.Write(kLogSpacer)
	w.Write([]byte(appOS))
	spaces := len(kLogSpaces) - len(appOS)
	if spaces > 0 {
		w.Write(kLogSpaces[:spaces])
	}
	w.Write([]byte(agentStr))
	w.Write(kLogEOL)
}

// ServeHTTP serves URLs with thw following format:
// {OrgID}.server.com/reflect
// {OrgID}.server.com[/.well-known]/{json-filename}
// {OrgID}.server.com/link/{link}
func (rs *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	if rs.statusErr != 0 {
		http.Error(w, "Internal error", rs.statusErr)
		return
	}

	orgDomain := req.Host
	reqPath := req.URL.Path

	// LEGACY
	if reqPath == "/VarSpace.json" {
		orgDomain = DefaultOrg
	}

	// Get the requested org (or bail)
	org := rs.orgs[orgDomain]
	if org == nil {
		http.NotFound(w, req)
		return
		//org = rs.orgs[DefaultOrg]
	}

	rs.logReq(req)

	if len(reqPath) > 0 && reqPath[0] == '/' {
		reqPath = reqPath[1:]
	}
	splitAt := strings.IndexRune(reqPath, '/')
	var verb, verbOp string
	if splitAt > 0 {
		verb = reqPath[:splitAt]
		verbOp = reqPath[splitAt+1:]
	}

	if verb == ".well-known" {
		jsonBuf, found := org.WellKnown[verbOp]
		if !found {
			http.NotFound(w, req)
			return
		}

		hdr := w.Header()
		hdr.Set("Content-Type", "application/json")
		hdr.Set("Content-Length", strconv.FormatInt(int64(len(jsonBuf)), 10))
		hdr.Set("Last-Modified", org.ModTime)
		hdr.Set("Server", ServerDesc)
		w.WriteHeader(http.StatusOK)
		w.Write(jsonBuf)
		return
	}

	if verb == "reflect" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, "orgDomain:'%v'\n", orgDomain)
		fmt.Fprintf(w, "req.Host:'%v'\n", req.Host)
		for k, v := range req.Header {
			fmt.Fprintf(w, "%v:'%v'\n", k, v)
		}
		return
	}

	if rs.serveLink(w, req, org, verb, verbOp) {
		return
	}

	// www
	rs.serveFile(w, req, reqPath, org)
}

type LinkParams struct {
	LinkDesc    string
	AppDomain   string
	AppDesc     string
	AppLink     string
	AppDownload string
	AppOS       string
}

func (rs *Server) serveLink(
	w http.ResponseWriter,
	req *http.Request,
	org *OrgService,
	verb string,
	reqLink string,
) bool {

	relLink := false
	if verb == ".link" {
		relLink = true
	} else if verb != "link" {
		return false
	}

	var appOS string
	agentStr := req.Header.Get("User-Agent")
	agent := useragent.Parse(agentStr)
	if agent != nil {
		appOS = agent.OS
	}

	var linkBuf strings.Builder
	linkBuf.WriteString("plan://")
	if relLink {
		linkBuf.WriteString(org.AppVars.AppDomain)
		linkBuf.WriteByte('/')
	}
	linkBuf.WriteString(reqLink)

	link := LinkParams{
		LinkDesc:    org.AppVars.AppDesc,
		AppDesc:     org.AppVars.AppDesc,
		AppDomain:   org.AppVars.AppDomain,
		AppLink:     linkBuf.String(),
		AppDownload: org.AppVars.AppDownloadURLs[appOS],
		AppOS:       appOS,
	}

	rs.logAgent(req, appOS, agentStr)

	w.Header().Set("Cache-Control", "no-store")
	rs.serveTemplate(w, org.linkTmpl, link)

	return true
}

func (rs *Server) serveTemplate(
	w http.ResponseWriter,
	tmpl *template.Template,
	vars interface{},
) {

	hdr := w.Header()
	hdr.Set("Content-Type", "text/html; charset=utf-8")
	hdr.Set("Last-Modified", rs.orgs[DefaultOrg].ModTime)
	hdr.Set("Server", ServerDesc)
	w.WriteHeader(http.StatusOK)

	err := tmpl.Execute(w, vars)
	if err != nil {
		log.Println(err)
		rs.statusErr = http.StatusInternalServerError
	}
}

func (rs *Server) serveFile(
	w http.ResponseWriter,
	req *http.Request,
	reqPath string,
	org *OrgService,
) {

	if reqPath == "" || reqPath == "/" {
		reqPath = "index.html"
	}

	rs.wwwMu.RLock()
	entry, ok := rs.www[reqPath]
	rs.wwwMu.RUnlock()

	if !ok {
		http.NotFound(w, req)
		return
	}

	if entry.recache() {
		rs.wwwMu.Lock()
		entry, ok = rs.www[reqPath]
		if !ok {
			http.NotFound(w, req)
			return
		}
		if entry.recache() {
			var err error
			entry.FileData, err = ioutil.ReadFile(path.Join(rs.wwwPath, reqPath))
			if err == nil {
				entry.FileSz = int64(len(entry.FileData))
				rs.www[reqPath] = entry
			}
		}
		rs.wwwMu.Unlock()
	}

	hdr := w.Header()

	// @TODO, if a file is larger than the cache limit, an error will result.
	// Basically, we just to load a serve that file via Read/w.Write loop
	if entry.tmpl == nil {
		bufSz := int64(len(entry.FileData))
		if entry.FileSz != bufSz {
			http.Error(w, "File system error", http.StatusInternalServerError)
			return
		}
		hdr.Set("Content-Length", strconv.FormatInt(bufSz, 10))
	}

	hdr.Set("Content-Type", entry.ContentType)
	hdr.Set("Last-Modified", entry.ModTime)
	hdr.Set("Server", ServerDesc)
	w.WriteHeader(http.StatusOK)

	if entry.tmpl != nil {
		err := entry.tmpl.Execute(w, org)
		if err != nil {
			log.Println(err)
			rs.statusErr = http.StatusInternalServerError
		}
	} else {
		w.Write(entry.FileData)
	}
}
