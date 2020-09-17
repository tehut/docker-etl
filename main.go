package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/lib/pq"
)

const (
	registryBaseURL = "https://hub.docker.com/v2/"
	connString      = "postgres://statuser:dev@localhost/dockerstats?sslmode=disable" //postgres:/localhost/dockerstats?sslmode=disable"
)

var maxValue = "100" //os.Getenv("MAX_VALUE")

type Repository struct {
	User        string
	Name        string
	Description string `json:description`
	PullCount   int    `json:"pull_count"`
	StarCount   int    `json:"star_count"`
	Lastupdated string `json:"last_updated"`
	CoreProduct string
}

type User struct {
	Count   int
	Next    string
	Results []Repository
}
type DB struct {
	*gorm.DB
}

func NewDB(connString string) (*sql.DB, error) {
	const (
		host     = "localhost"
		port     = 5432
		user     = "statuser"
		password = "dev"
		dbname   = "dockerstats"
	)

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	// "statuser:dev@localhost/dockerstats?sslmode=disable" -- not clear why this connection string
	// is failing with ssl mode not enabled
	// ("postgres",	"statuser:dev@localhost/dockerstats?sslmode=disable"
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func FindSub(check []string, item string) bool {
	found := false
	for _, v := range check {

		if strings.Contains(strings.ToLower(item), v) {
			found = true
		}
	}
	return found
}

func SortValues(resultStruct User) map[string][]Repository {
	sortMap := map[string][]Repository{
		"consul":    make([]Repository, 0),
		"packer":    make([]Repository, 0),
		"nomad":     make([]Repository, 0),
		"terraform": make([]Repository, 0),
		"vagrant":   make([]Repository, 0),
		"vault":     make([]Repository, 0),
		"other":     make([]Repository, 0),
	}
	//whitelist of keywords that indicate a core product container
	keepKeys := []string{"official", "first-class", "automatic", "builds", "source", "jsii"}

	for _, repo := range resultStruct.Results {
		productLoops := 0

		for product, slice := range sortMap {
			productLoops++
			found := FindSub(keepKeys, repo.Description)

			// if a repo description doesn't contain any of the keywords for a blessed
			// image or the name doesn't include "enterprise", mark as other & end loop
			if !found && !strings.Contains(repo.Name, "enterprise") {
				other := append(sortMap["other"], repo)
				sortMap["other"] = other
				break
			}

			// for repos idenitfied as blessed, save to appropriate
			// core product slice
			if strings.Contains(repo.Name, product) {
				repo.CoreProduct = product
				slice = append(slice, repo)
				sortMap[product] = slice
			}
		}
	}
	return sortMap
}

func CheckMap(resultStruct User, sortedMap map[string][]Repository) error {
	// Count sorted repos to verify all are accounted for
	total := 0
	for _, slice := range sortedMap {
		total = total + len(slice)
	}
	if total != resultStruct.Count {
		err := fmt.Errorf("verification failure: sorted repository count does not match full count of Hashicorp dockerhub repositories. \n Sourted Count: %d, Hashicorp Repos:%d \n\n", total, resultStruct.Count)
		return err
	}
	//fmt.Printf("%+v\n", sortedMap) // for debugging, temporary
	return nil
}

func InsertData(repo Repository, trans *sql.Tx, now string) error {
	var updated *string
	if len(repo.Lastupdated) <= 1 {
		updated = nil
	} else {
		updated = &repo.Lastupdated
	}

	_, err := trans.Exec("INSERT INTO docker(repo_name, pull_count, star_count, last_updated, recorded_date, core_product) VALUES ($1, $2, $3, $4, $5, $6)",
		repo.Name, repo.PullCount, repo.StarCount, updated, now, repo.CoreProduct)

	if err != nil {
		trans.Rollback()
		log.Fatal(err)
		return nil
	}
	return nil
}
func runInserts(productMap map[string][]Repository, db *sql.DB) {
	time := time.Now().UTC().Format(time.RFC3339)
	fmt.Println(time)
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	for _, array := range productMap {
		for _, list := range array {
			InsertData(list, tx, time)
		}
	}
	tx.Commit()
	tx.Commit()

	return
}
func main() {
	userURL := registryBaseURL + "repositories/hashicorp/?page_size=" + maxValue
	resp, err := retryablehttp.Get(userURL)
	if err != nil {
		fmt.Println(err.Error())
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err.Error())
	}

	hashiUser := User{}
	json.Unmarshal(b, &hashiUser)
	productMap := SortValues(hashiUser)
	err = CheckMap(hashiUser, productMap)
	if err != nil {
		fmt.Println(err)
	}
	db, err := NewDB(connString)
	if err != nil {
		log.Fatal(err)
	}
	runInserts(productMap, db)

	var (
		name        string
		pullCount   int
		starCount   int
		lastUpdated string
	)
	rows, err := db.Query("SELECT * FROM docker")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		err := rows.Scan(&name, &pullCount, &starCount, &lastUpdated)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf(" %s, %s, %s, %s\n", name, pullCount, starCount, lastUpdated)
	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}

}
