package publish

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	_ "github.com/lib/pq"
	"github.com/urfave/cli"

	record "github.com/rancher/telemetry/record"
)

type Postgres struct {
	telemetryVersion string

	Conn *sql.DB
}

type ApiInstallation struct {
	Id        int64       `json:"id"`
	Uid       string      `json:"uid"`
	FirstSeen time.Time   `json:"first_seen"`
	LastSeen  time.Time   `json:"last_seen"`
	Record    interface{} `json:"record"`
}

type ApiRecord struct {
	Id     int64       `json:"id"`
	Uid    string      `json:"uid"`
	Ts     time.Time   `json:"ts"`
	Record interface{} `json:"record"`
}

type RecordsByUid map[string]ApiRecord
type RecordsByDateByUid map[string]RecordsByUid

func NewPostgres(c *cli.Context) *Postgres {
	host := c.String("pg-host")
	port := c.String("pg-port")
	user := c.String("pg-user")
	pass := c.String("pg-pass")
	dbname := c.String("pg-dbname")
	sslmode := c.String("pg-ssl")

	out := &Postgres{
		telemetryVersion: c.App.Version,
	}

	if host != "" && user != "" && pass != "" {
		log.Info("Postgres enabled")
	} else {
		return out
	}

	dsn := strings.Join([]string{
		"host=" + host,
		"port=" + port,
		"user=" + user,
		"password='" + pass + "'",
		"dbname=" + dbname,
		"sslmode=" + sslmode,
	}, " ")

	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Error connecting to DB: %s", err)
	}

	out.Conn = conn
	err = out.testDb()
	if err != nil {
		log.Fatalf("Error connecting to DB: %s", err)
	}

	log.Infof("Connected to Postgres at %s", host)
	return out
}

func (p *Postgres) Report(r record.Record, clientIp string) error {
	log.Debugf("Publishing to Postgres")

	install := r["install"].(map[string]interface{})
	uid := install["uid"].(string)

	recordId, err := p.addRecord(uid, r)
	log.Debugf("Add Record: %s, %s", recordId, err)
	if err != nil {
		log.Debugf("Error writing to DB: %s", err)
		return err
	}

	_, err = p.upsertInstall(uid, clientIp, recordId)
	if err != nil {
		log.Debugf("Error writing to DB: %s", err)
		return err
	}

	log.Debugf("Published to Postgres")
	return nil
}

func (p *Postgres) testDb() error {
	var one int
	err := p.Conn.QueryRow(`SELECT 1`).Scan(&one)
	if err != nil {
		return err
	}

	if one != 1 {
		return errors.New(fmt.Sprintf("SELECT 1 == %d?!", one))
	}

	return nil
}

func (p *Postgres) addRecord(uid string, r record.Record) (int, error) {
	var id int

	b, err := json.Marshal(r)
	if err != nil {
		return 0, err
	}

	err = p.Conn.QueryRow(`INSERT INTO record(uid,data,ts) VALUES ($1,$2,NOW()) RETURNING id`, uid, string(b)).Scan(&id)
	return id, err
}

func (p *Postgres) upsertInstall(uid string, clientIp string, recordId int) (int, error) {
	var id int

	err := p.Conn.QueryRow(`
		INSERT INTO installation(uid,last_ip,last_record,first_seen,last_seen)
		VALUES ($1,$2,$3,NOW(),NOW()) 
		ON CONFLICT(uid) DO UPDATE SET 
			last_seen=NOW(),
			last_ip=$2,
			last_record=$3
		RETURNING id`, uid, clientIp, recordId).Scan(&id)
	return id, err
}

func (p *Postgres) GetLatest(hours int) ([]ApiInstallation, error) {
	sql := `
		SELECT i.id, i.uid, i.first_seen, i.last_seen, r.data
		FROM installation i
			JOIN record r ON (i.last_record = r.id)
		WHERE i.last_seen >= NOW() - INTERVAL '%d hour'`

	rows, err := p.Conn.Query(fmt.Sprintf(sql, hours))

	defer rows.Close()

	if err != nil {
		return nil, err
	}

	out := []ApiInstallation{}

	defer rows.Close()
	for rows.Next() {
		var i ApiInstallation
		var data []byte
		err = rows.Scan(&i.Id, &i.Uid, &i.FirstSeen, &i.LastSeen, &data)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(data, &i.Record)
		if err != nil {
			return nil, err
		}

		out = append(out, i)
	}

	return out, nil
}

func (p *Postgres) GetByDay(hours int) (RecordsByDateByUid, error) {
	sql := `
		SELECT id, uid, ts, data
		FROM record
		WHERE ts >= NOW() - INTERVAL '%d hour'
		ORDER BY id DESC`

	rows, err := p.Conn.Query(fmt.Sprintf(sql, hours))

	defer rows.Close()

	if err != nil {
		return nil, err
	}

	days := make(RecordsByDateByUid)

	defer rows.Close()
	for rows.Next() {
		var rec ApiRecord
		var data []byte
		err = rows.Scan(&rec.Id, &rec.Uid, &rec.Ts, &data)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(data, &rec.Record)
		if err != nil {
			return nil, err
		}

		day := rec.Ts.Format("2006-01-02")
		byDate, ok := days[day]
		if !ok {
			byDate = make(RecordsByUid)
			days[day] = byDate
		}

		_, exists := byDate[rec.Uid]
		if !exists {
			byDate[rec.Uid] = rec
		}
	}

	return days, nil
}

func (p *Postgres) GetRecordsByUid(uid string, hours int) ([]ApiRecord, error) {
	sql := `
		SELECT id, uid, ts, data
		FROM record
		WHERE 
			uid = $1
			AND ts >= NOW() - INTERVAL '%d hour'`

	rows, err := p.Conn.Query(fmt.Sprintf(sql, hours), uid)

	if err != nil {
		return nil, err
	}

	out := []ApiRecord{}

	defer rows.Close()
	for rows.Next() {
		var rec ApiRecord
		var data []byte
		err = rows.Scan(&rec.Id, &rec.Uid, &rec.Ts, &data)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(data, &rec.Record)
		if err != nil {
			return nil, err
		}

		out = append(out, rec)
	}

	return out, nil
}

func (p *Postgres) GetRecordById(id string) (ApiRecord, error) {
	sql := `
		SELECT id, uid, ts, data
		FROM record
		WHERE 
			id = $1`

	var rec ApiRecord
	var data []byte

	err := p.Conn.QueryRow(sql, id).Scan(&rec.Id, &rec.Uid, &rec.Ts, &data)
	if err != nil {
		return rec, err
	}

	err = json.Unmarshal(data, &rec.Record)
	if err != nil {
		return rec, err
	}

	return rec, nil
}