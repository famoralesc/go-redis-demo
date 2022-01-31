package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"net/http"
	"net/url"
	"os"
	"time"
)

type APIResponse struct {
	Cache bool                `json:"cache"`
	Data  []NominatimResponse `json:"data"`
}

type NominatimResponse struct {
	PlaceID     int      `json:"place_id"`
	Licence     string   `json:"licence"`
	OsmType     string   `json:"osm_type"`
	OsmID       int      `json:"osm_id"`
	Boundingbox []string `json:"boundingbox"`
	Lat         string   `json:"lat"`
	Lon         string   `json:"lon"`
	DisplayName string   `json:"display_name"`
	Class       string   `json:"class"`
	Type        string   `json:"type"`
	Importance  float64  `json:"importance"`
	Icon        string   `json:"icon"`
}

type API struct {
	cache *redis.Client
}

func main() {
	fmt.Println("Starting Server")

	api := NewAPI()
	http.HandleFunc("/api", api.Handler)

	http.ListenAndServe(fmt.Sprintf(":%s", os.Getenv("PORT")), nil)
}

func (a *API) GetCachedData(ctx context.Context, key string) string {

	val, err := a.cache.Get(ctx, key).Result()
	if err == redis.Nil {
		return ""
	} else if err != nil {
		fmt.Println("error calling redis: %w", err)
		return ""
	}
	return val
}

func (a *API) Handler(w http.ResponseWriter, r *http.Request) {
	println("In Handler")
	q := r.URL.Query().Get("q")

	data, cacheHit, err := a.getData(r.Context(), q)
	if err != nil {
		fmt.Println("Error calling data source : %w", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	response := APIResponse{
		Cache: cacheHit,
		Data:  data,
	}
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		fmt.Println("error encoding response: %w", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (a *API) getData(ctx context.Context, q string) ([]NominatimResponse, bool, error) {
	// get cached data first
	escapedQ := url.PathEscape(q)

	cachedData := a.GetCachedData(ctx, escapedQ)

	var data []NominatimResponse

	isCached := false
	if cachedData != "" {
		// use the cached to be decoded
		err := json.Unmarshal(bytes.NewBufferString(cachedData).Bytes(), &data)
		if err != nil {
			return nil, isCached, err
		}
		isCached = true

	} else {
		address := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=%s&format=json", escapedQ)
		response, err := http.Get(address)
		if err != nil {
			return nil, isCached, err
		}

		err = json.NewDecoder(response.Body).Decode(&data)
		if err != nil {
			return nil, isCached, err
		}
		b, err := json.Marshal(data)

		if err != nil {
			return nil, isCached, err
		}

		//set the value
		err = a.cache.Set(ctx, escapedQ, bytes.NewBuffer(b).Bytes(), time.Second*15).Err()
		if err != nil {
			return nil, isCached, err
		}
	}

	return data, isCached, nil
}

func NewAPI() *API {
	var opts *redis.Options
	var err error

	if os.Getenv("LOCAL") == "true" {
		redisAddress := fmt.Sprintf("%s:6379", os.Getenv("REDIS_URL"))
		opts = &redis.Options{
			Addr:     redisAddress,
			Password: "",
			DB:       0,
		}
	} else {
		opts, err = redis.ParseURL(os.Getenv("REDIS_URL"))
		if err != nil {
			panic(err)
		}
	}

	rdb := redis.NewClient(opts)

	return &API{
		cache: rdb,
	}
}
