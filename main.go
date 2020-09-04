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
		if strings.Contains(item, v) {
			found = true
		}
	}
	return found
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
	productMap := map[string][]Repository{
		"consul":    make([]Repository, 0),
		"packer":    make([]Repository, 0),
		"nomad":     make([]Repository, 0),
		"terraform": make([]Repository, 0),
		"vagrant":   make([]Repository, 0),
		"vault":     make([]Repository, 0),
		"other":     make([]Repository, 0),
	}
	// keywords that indicate a peripherals container, not the product
	dropKeys := []string{
		"replicate",
		"template",
		"env",
		"controller",
		"website",
		"broker",
		"driver",
		"demo",
		"logging",
		"autoscaler",
		"spark",
	}

	// sort repos into core product groups
	for _, repo := range hashiUser.Results {
		productLoops := 0

		for product, slice := range productMap {
			productLoops = productLoops + 1

			if found := FindSub(dropKeys, repo.Name); found {
				other := append(productMap["other"], repo)
				productMap["other"] = other
				break
			}

			if strings.Contains(repo.Name, product) {
				repo.CoreProduct = product
				slice = append(slice, repo)
				productMap[product] = slice
				break
			}

			if productLoops > 6 {
				other := append(productMap["other"], repo)
				productMap["other"] = other
			}
		}

	}
	// Count sorted repos to verify all are accounted for
	total := 0
	for _, slice := range productMap {
		total = total + len(slice)
	}
	if total != hashiUser.Count {
		fmt.Printf("Verification failure: sorted repository count does not match full count of Hashicorp dockerhub repositories. \n Sourted Count: %d, Hashicorp Repos:%d \n\n", total, hashiUser.Count)
	}
	fmt.Printf("%+v\n", productMap)
}
