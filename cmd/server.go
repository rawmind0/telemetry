package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/goji/httpauth"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/urfave/cli"

	publish "github.com/rancher/telemetry/publish"
	record "github.com/rancher/telemetry/record"
)

const DEF_HOURS = 7
const DEF_DAYS = 28

var (
	version         string
	enableXff       bool
	googlePublisher *publish.Google
	dbPublisher     *publish.Postgres
)

type RequiredOptions []string

type RequestOpts struct {
	Hours  int
	Days   int
	Uid    string
	Fields []string
	Field  string
}

func ServerCommand() cli.Command {
	return cli.Command{
		Name:   "server",
		Usage:  "gather stats from a telemetry client",
		Action: serverRun,
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "listen, l",
				Usage: "address/port to listen on",
				Value: "0.0.0.0:8115",
			},

			cli.BoolFlag{
				Name:        "xff",
				Usage:       "enable support for X-Forwarded-For header",
				Destination: &enableXff,
			},

			cli.StringFlag{
				Name:   "ga-tid",
				Usage:  "google analytics tracking id",
				Value:  "",
				EnvVar: "TELEMETRY_GA_TID",
			},

			cli.StringFlag{
				Name:   "pg-host",
				Usage:  "postgres host",
				Value:  "localhost",
				EnvVar: "TELEMETRY_PG_HOST",
			},
			cli.StringFlag{
				Name:   "pg-port",
				Usage:  "postgres port",
				Value:  "5432",
				EnvVar: "TELEMETRY_PG_PORT",
			},
			cli.StringFlag{
				Name:   "pg-user",
				Usage:  "postgres user",
				Value:  "telemetry",
				EnvVar: "TELEMETRY_PG_USER",
			},
			cli.StringFlag{
				Name:   "pg-pass",
				Usage:  "postgres password",
				Value:  "",
				EnvVar: "TELEMETRY_PG_PASS",
			},
			cli.StringFlag{
				Name:   "pg-dbname",
				Usage:  "postgres dbname",
				Value:  "telemetry",
				EnvVar: "TELEMETRY_PG_DBNAME",
			},
			cli.StringFlag{
				Name:   "pg-ssl",
				Usage:  "postgres ssl mode (disable, require, verify-ca, verify-full)",
				Value:  "disable",
				EnvVar: "TELEMETRY_PG_SSL",
			},

			cli.StringFlag{
				Name:   "admin-key",
				Usage:  "admin access key",
				Value:  "",
				EnvVar: "TELEMETRY_API_KEY",
			},

			cli.StringFlag{
				Name:   "admin-secret",
				Usage:  "admin secret key",
				Value:  "",
				EnvVar: "TELEMETRY_SECRET_KEY",
			},
		},
	}
}

func serverRun(c *cli.Context) error {
	log.Infof("Telemetry Server %s", c.App.Version)
	rand.Seed(time.Now().UnixNano())

	version = c.App.Version
	googlePublisher = publish.NewGoogle(c)
	dbPublisher = publish.NewPostgres(c)

	router := mux.NewRouter()
	router.HandleFunc("/favicon.ico", http.NotFound)
	router.HandleFunc("/healthcheck.html", serverCheck).Methods("GET")
	router.HandleFunc("/publish", serverPublish).Methods("POST")
	router.HandleFunc("/", serverRoot).Methods("GET")

	user := c.String("admin-key")
	pass := c.String("admin-secret")
	if user == "" || pass == "" {
		log.Warn("admin-{key,-secret} not set, admin disabled")
	} else {
		admin := mux.NewRouter()
		//	admin.HandleFunc("/admin/net-installs", ...)                                // ?days=28

		admin.HandleFunc("/admin/active", apiActive)                       // ?hours=7
		admin.HandleFunc("/admin/active/fields/{fields}", apiActiveFields) // ?hours=7
		admin.HandleFunc("/admin/active/map/{field}", apiActiveMap)        // ?hours=7
		admin.HandleFunc("/admin/active/value/{field}", apiActiveValue)    // ?hours=7

		admin.HandleFunc("/admin/history", apiHistory)                       // ?days=28
		admin.HandleFunc("/admin/history/fields/{fields}", apiHistoryFields) // ?days=28
		admin.HandleFunc("/admin/history/map/{field}", apiHistoryMap)        // ?days=28
		admin.HandleFunc("/admin/history/value/{field}", apiHistoryValue)    // ?days=28

		admin.HandleFunc("/admin/installs/{uid}", apiInstallByUid)                  // ?days=28
		admin.HandleFunc("/admin/installs/{uid}/fields/{fields}", apiInstallFields) // ?days=28
		admin.HandleFunc("/admin/installs/{uid}/map/{field}", apiInstallMap)        // ?days=28
		admin.HandleFunc("/admin/installs/{uid}/value/{field}", apiInstallValue)    // ?days=28

		admin.HandleFunc("/admin/records/{id}", apiRecordById) // nothing

		authed := httpauth.SimpleBasicAuth(user, pass)(admin)

		router.Handle("/admin", authed)
		router.Handle("/admin/{_dummy:.*}", authed)
	}

	cors := handlers.CORS(
		handlers.AllowedHeaders([]string{"authorization"}),
	)(router)

	logged := handlers.LoggingHandler(os.Stdout, cors)

	listen := c.String("listen")
	log.Info("Listening on ", listen)
	log.Fatal(http.ListenAndServe(listen, logged))
	return nil
}

func serverCheck(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("pageok"))
}

func abs(i int) int {
	if i < 0 {
		return -1 * i
	}

	return i
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

func max(i, j int) int {
	if i > j {
		return i
	}
	return j
}

func clamp(i, x, j int) int {
	return max(i, min(x, j))
}

func round(f float64) int {
	return int(f + 0.5)
}

func serverRoot(w http.ResponseWriter, req *http.Request) {
	nRows := 15
	nCols := 80

	var rows [][]byte
	for y := 0; y < nRows; y++ {
		rows = append(rows, make([]byte, nCols+1, nCols+1))

		for x := 0; x <= nCols; x++ {
			if x == 0 || x == nCols-1 {
				rows[y][x] = '|'
			} else if x == nCols {
				rows[y][x] = '\n'
			} else {
				rows[y][x] = ' '
			}
		}
	}

	y := nRows / 2
	ly := y
	dy := 0.0
	for x := 3; x < nCols-2; x++ {
		log.Debugf("y=%d, ly=%d, dy=%f", y, ly, dy)
		rows[y][x] = 'X'
		rows[y][x-1] = 'X'
		diff := abs(ly - y)
		y1 := min(ly, y) + 1
		y2 := max(ly, y)
		if diff > 1 {
			for z := y1; z < y2; z++ {
				rows[z][x] = 'X'
				rows[z][x-1] = 'X'
			}
		}

		dy += float64(rand.Int()%10)/10.0 - 0.5
		if dy < -2.0 {
			dy = -2.0
		} else if dy > 2.0 {
			dy = 2.0
		}

		ly = y
		y = round(float64(y) + dy)
		if y < 0 {
			y = 0
			dy = -dy
		} else if y > nRows-1 {
			y = nRows - 1
			dy = -dy
		}
	}

	w.Write([]byte(fmt.Sprintf("Rancher Telemetry %s\n", version)))
	w.Write([]byte("+" + strings.Repeat("-", nCols-2) + "+\n"))
	w.Write([]byte("|" + strings.Repeat(" ", nCols-2) + "|\n"))

	for _, row := range rows {
		w.Write(row)
	}

	w.Write([]byte("|" + strings.Repeat(" ", nCols-2) + "|\n"))
	w.Write([]byte("+" + strings.Repeat("-", nCols-2) + "+\n"))
}

func serverPublish(w http.ResponseWriter, req *http.Request) {
	var r record.Record

	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&r)
	if err != nil {
		respondError(w, req, "Error parsing Record", 400)
		return
	}

	realIp := requestIp(req)
	ip := anonymizeIp(realIp)
	log.Debugf("Publish from %s: %s", realIp, r)

	err = googlePublisher.Report(r, ip)
	if err != nil {
		log.Errorf("Error publishing to Google: %s", err)
	}

	dbPublisher.Report(r, ip)
	if err != nil {
		log.Errorf("Error publishing to DB: %s", err)
	}

	respondSuccess(w, req, map[string]string{"ok": "1"})
}

// ------------
// Counts
// ------------
func getFields(w http.ResponseWriter, req *http.Request, which string) {
	var out interface{}
	var err error

	opt, err := getOptions(req, RequiredOptions{"Fields"})
	if err != nil {
		respondError(w, req, err.Error(), 422)
		return
	}

	switch which {
	case "active":
		out, err = dbPublisher.SumOfActiveInstalls(opt.Hours, opt.Fields)
	case "history":
		out, err = dbPublisher.SumByDay(opt.Days, opt.Fields, "")
	case "install":
		out, err = dbPublisher.SumByDay(opt.Days, opt.Fields, opt.Uid)
	default:
		respondError(w, req, "Invalid which", 400)
		return
	}

	respond(w, req, out, err)
}

func getMap(w http.ResponseWriter, req *http.Request, which string) {
	var out interface{}
	var err error

	opt, err := getOptions(req, RequiredOptions{"Field"})
	if err != nil {
		respondError(w, req, err.Error(), 422)
		return
	}

	switch which {
	case "active":
		out, err = dbPublisher.SumOfActiveInstallsMap(opt.Hours, opt.Field)
	case "history":
		out, err = dbPublisher.SumByDayMap(opt.Days, opt.Field, "")
	case "install":
		out, err = dbPublisher.SumByDayMap(opt.Days, opt.Field, opt.Uid)
	default:
		respondError(w, req, "Invalid which", 400)
		return
	}

	respond(w, req, out, err)
}

func getValue(w http.ResponseWriter, req *http.Request, which string) {
	var out interface{}
	var err error

	opt, err := getOptions(req, RequiredOptions{"Field"})
	if err != nil {
		respondError(w, req, err.Error(), 422)
		return
	}

	switch which {
	case "active":
		out, err = dbPublisher.SumOfActiveInstallsValue(opt.Hours, opt.Field)
	case "history":
		out, err = dbPublisher.SumByDayValue(opt.Days, opt.Field, "")
	case "install":
		out, err = dbPublisher.SumByDayValue(opt.Days, opt.Field, opt.Uid)
	default:
		respondError(w, req, "Invalid which", 400)
		return
	}

	respond(w, req, out, err)
}

// ------------
// Active
// ------------
func apiActive(w http.ResponseWriter, req *http.Request) {
	opt, err := getOptions(req, RequiredOptions{})
	if err != nil {
		respondError(w, req, err.Error(), 422)
		return
	}

	installs, err := dbPublisher.GetActiveInstalls(opt.Hours)
	if err != nil {
		respondError(w, req, err.Error(), 500)
		return
	}

	coll := Collection{
		Type:         "collection",
		ResourceType: "installation",
		Data:         installs,
	}

	respondSuccess(w, req, coll)
}

func apiActiveFields(w http.ResponseWriter, req *http.Request) {
	getFields(w, req, "active")
}

func apiActiveMap(w http.ResponseWriter, req *http.Request) {
	getMap(w, req, "active")
}

func apiActiveValue(w http.ResponseWriter, req *http.Request) {
	getValue(w, req, "active")
}

// ------------
// History
// ------------
func apiHistory(w http.ResponseWriter, req *http.Request) {
	opt, err := getOptions(req, RequiredOptions{})
	if err != nil {
		respondError(w, req, err.Error(), 422)
		return
	}

	out, err := dbPublisher.GetRecordsGroupedByDay(opt.Days)
	respond(w, req, out, err)
}

func apiHistoryFields(w http.ResponseWriter, req *http.Request) {
	getFields(w, req, "history")
}

func apiHistoryMap(w http.ResponseWriter, req *http.Request) {
	getMap(w, req, "history")
}

func apiHistoryValue(w http.ResponseWriter, req *http.Request) {
	getValue(w, req, "history")
}

// ------------
// By Install
// ------------
func apiInstallByUid(w http.ResponseWriter, req *http.Request) {
	opt, err := getOptions(req, RequiredOptions{"Uid"})

	records, err := dbPublisher.GetRecordsByUid(opt.Uid, opt.Days)
	if err != nil {
		respondError(w, req, err.Error(), 500)
		return
	}

	coll := Collection{
		Type:         "collection",
		ResourceType: "record",
		Data:         records,
	}

	respondSuccess(w, req, coll)
}

func apiInstallFields(w http.ResponseWriter, req *http.Request) {
	getFields(w, req, "install")
}

func apiInstallMap(w http.ResponseWriter, req *http.Request) {
	getMap(w, req, "install")
}

func apiInstallValue(w http.ResponseWriter, req *http.Request) {
	getValue(w, req, "install")
}

// ------------
// By Record
// ------------
func apiRecordById(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	id := vars["id"]

	if id == "" {
		respondError(w, req, "ID is required", 422)
		return
	}

	out, err := dbPublisher.GetRecordById(id)
	respond(w, req, out, err)
}

func adminUi(w http.ResponseWriter, req *http.Request) {
	respondSuccess(w, req, "<html><body>Hi</body></html>")
}

// ------------

func requestIp(req *http.Request) string {
	if enableXff {
		clientIp := req.Header.Get("X-Forwarded-For")
		if len(clientIp) > 0 {
			return clientIp
		}
	}

	clientIp, _, _ := net.SplitHostPort(req.RemoteAddr)
	return clientIp
}

func anonymizeIp(in string) string {
	ip := net.ParseIP(in).To16()
	if ip == nil {
		return in
	}

	var mask net.IPMask
	v4 := ip.To4()
	if v4 == nil {
		mask = net.CIDRMask(8*8, 8*16)
	} else {
		mask = net.CIDRMask(8*3, 8*4)
	}

	return ip.Mask(mask).String()
}

func getOptions(req *http.Request, required RequiredOptions) (RequestOpts, error) {
	out := RequestOpts{
		Hours: DEF_HOURS,
		Days:  DEF_DAYS,
	}

	str := req.URL.Query().Get("hours")
	if str != "" {
		num, err := strconv.Atoi(str)
		if err == nil {
			out.Hours = num
		}
	}

	str = req.URL.Query().Get("days")
	if str != "" {
		num, err := strconv.Atoi(str)
		if err == nil {
			out.Days = num
		}
	}

	vars := mux.Vars(req)
	out.Fields = strings.Split(vars["fields"], ",")
	out.Field = vars["field"]
	out.Uid = vars["uid"]

	if out.Hours < 1 {
		return out, errors.New("Hours must be > 0")
	}

	if out.Days < 1 {
		return out, errors.New("Days must be > 0")
	}

	if required != nil {
		if required.Contains("Uid") && len(out.Uid) == 0 {
			return out, errors.New("You must provide a field")
		}

		if required.Contains("Fields") && len(out.Fields) == 0 {
			return out, errors.New("You must provide some fields")
		}

		if required.Contains("Field") && out.Field == "" {
			return out, errors.New("You must provide a field")
		}
	}

	return out, nil
}

func (r *RequiredOptions) Contains(needle string) bool {
	needle = strings.ToLower(needle)
	for _, val := range *r {
		if strings.ToLower(val) == needle {
			return true
		}
	}

	return false
}
