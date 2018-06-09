package main

import (
	"log"
	"net/http"
	"fmt"
	"os"
	"database/sql"
	"github.com/cloudfoundry-community/go-cfenv"
	_ "github.com/lib/pq"
	"strings"
	_ "github.com/go-sql-driver/mysql"
)

type server struct {
	DB *sql.DB
}

// Cfg struct
var Cfg = CfgType{}

// CfgType for mysql default database
type CfgType struct {
	Username      string
	Password      string
	Hostname      string
	Port          string
	DbType        string
	isInMemory    bool
	Database      string
	ConnectString string
	AdminConnect  string
	Tablename     string
	CharType      string
}





func (server *server) CreateHTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/bootstrap", server.bootstrap)
	mux.HandleFunc("/favicon.ico", http.NotFound)
	mux.HandleFunc("/", server.incrementer)
	return mux
}

func (server *server) bootstrap(w http.ResponseWriter, r *http.Request) {
	_, err := server.DB.Exec ("CREATE TABLE IF NOT EXISTS counter (name varchar(255), value integer);")
	if err != nil {
		fmt.Fprintf(w, "Error creating db: %s", err.Error())
		return
	}
}

func (server *server) incrementer(w http.ResponseWriter, r *http.Request) {
	ip := guessIPofRequester(r)
	fmt.Sprintf("ip = %s", ip)

	var curCount = 0
	err := server.DB.QueryRow("select value from counter where name = ?;", ip).Scan(&curCount)
	switch err {
	case nil:
		curCount++
		_, err = server.DB.Exec("update counter set value = ? where name = ?;", curCount, ip)
		if err != nil {
			log.Panic(err)
		}
	case sql.ErrNoRows:
		_, err = server.DB.Exec("insert into counter(name, value) values(?, ?);", ip, 1)
		if err != nil {
			log.Panic(err)
		}
	default:
		log.Panic(err)
	}
	fmt.Fprintf(w, "Hello %s. You have visited %d times.", ip, curCount)
}

func guessIPofRequester(r *http.Request) string {
	forwardedIPs, ok := r.Header["X-Forwarded-For"]
	if !ok {
		forwardedIPs = nil
	}

	// Since some proxies add comma's, and others headers, handle both
	forwardedIPs = strings.Split(strings.Join(forwardedIPs, ","), ",")

	// Last one added should be added by our reverse proxy
	for idx := len(forwardedIPs) - 1; idx >= 0; idx-- {
		ip := strings.TrimSpace(forwardedIPs[idx])
		if len(ip) == 0 {
			continue
		}
		if strings.HasPrefix(ip, "10.") { // but apparently we must be behind at least 2, so skip any 10. addresses
			// TODO - be more precise about this
			continue
		}
		return ip
	}
	return "unknown"
}

func getDBFromCF() (*sql.DB, error) {
	appEnv, err := cfenv.Current()
	if err != nil {
		return nil, err
	}

	boundServices, err := appEnv.Services.WithTag("mysql")
	if (err != nil) || (len(boundServices) <= 0) {
		log.Panic("====================================================================================")
		log.Panic("Error cannot find a bound service with tag mysql ...")
		log.Panic("====================================================================================")
	}

	dbParams := "autocommit=true"

	Cfg.DbType = "mysql"
	boundService := boundServices[0]
	Cfg.Username, _ = boundService.CredentialString("username")
	Cfg.Password, _ = boundService.CredentialString("password")
	Cfg.Hostname, _ = boundService.CredentialString("hostname")
	p, _ := boundService.Credentials["port"]
	Cfg.Port = fmt.Sprintf("%.0f", p)
	Cfg.Database, _ = boundService.CredentialString("name")
	Cfg.CharType = "VARCHAR(255)"
	Cfg.ConnectString = fmt.Sprintf("%v:%v@tcp(%v:%v)/%v?%v", Cfg.Username, Cfg.Password, Cfg.Hostname, Cfg.Port, Cfg.Database, dbParams)

	return sql.Open(Cfg.DbType, Cfg.ConnectString)
}

func main() {
	// Get the database
	db, err := getDBFromCF()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Start the app
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", os.Getenv("PORT")), (&server {
		DB: db,
	}).CreateHTTPHandler() ))
}


