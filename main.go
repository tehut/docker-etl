package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/jinzhu/gorm"
)

const registryBaseURL = "https://hub.docker.com/v2/"

var maxValue = os.Getenv("MAX_VALUE")

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

func NewDB(connString string) (*DB, error) {
	var outErr error
	for x := 0; x < 3; x++ {
		time.Sleep(time.Second)
		db, err := gorm.Open("postgres", connString)
		if err != nil {
			outErr = err
			continue
		}
		if err = db.DB().Ping(); err != nil {
			outErr = err
			continue
		}
		return &DB{
			DB: db,
		}, nil
	}
	return nil, outErr
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
	// keywords that indicate a peripherals container, not the product
	keepKeys := []string{"official", "first-class", "automatic", "builds", "source", "jsii"}

	for _, repo := range resultStruct.Results {
		productLoops := 0

		for product, slice := range sortMap {
			productLoops = productLoops + 1
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

func CheckMap(resultStruct User, sortedMap map[string][]Repository) {
	// Count sorted repos to verify all are accounted for
	total := 0
	for _, slice := range sortedMap {
		total = total + len(slice)
	}
	if total != resultStruct.Count {
		fmt.Printf("Verification failure: sorted repository count does not match full count of Hashicorp dockerhub repositories. \n Sourted Count: %d, Hashicorp Repos:%d \n\n", total, resultStruct.Count)
	}
	fmt.Printf("%+v\n", sortedMap)
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
	CheckMap(hashiUser, productMap)
}
