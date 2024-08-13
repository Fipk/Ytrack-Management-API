package main

import (
	"Ytrack-Manager/ApiInterface"
	"Ytrack-Manager/tools"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
)

var client *ApiInterface.Client

type Campus struct {
	Id       int                    `json:"id"`
	Name     string                 `json:"name"`
	Type     string                 `json:"type"`
	Attrs    map[string]interface{} `json:"attrs"`
	Children map[string]CampusChild `json:"children"`
}

type CampusChild struct {
	Id    int    `json:"id"`
	Name  string `json:"name"`
	Index int    `json:"index"`
}

func GetCampus(campusName string) (Campus, error) {
	var campus Campus

	resp, err := http.Get("https://ytrack.learn.ynov.com/api/object/" + campusName)
	if err != nil {
		return Campus{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Campus{}, err
	}
	temp := make(map[string]interface{})

	err = json.Unmarshal(body, &temp)
	if err != nil {
		return Campus{}, err
	}
	// catch the panic that occurs when the campusName is not found
	defer func() {
		if r := recover(); r != nil {
			log.Fatal("The campus name was not found, check the platform configuration file")
		}
	}()

	campus.Id = int(temp["id"].(float64))
	campus.Name = temp["name"].(string)
	campus.Type = temp["type"].(string)
	campus.Attrs = temp["attrs"].(map[string]interface{})
	campus.Children = make(map[string]CampusChild)

	if children, ok := temp["children"].(map[string]interface{}); ok {
		for key := range children {
			child := children[key].(map[string]interface{})
			campus.Children[key] = CampusChild{
				Id:    int(child["id"].(float64)),
				Name:  key,
				Index: int(child["index"].(float64)),
			}
		}
	}

	return campus, nil
}

//func GetUserCourses(userId int) ([]int, error) {
//
//}

func ExtractId(token string) (string, error) {
	payload, err := ApiInterface.Decode(token)
	if err != nil {
		return "", err
	}
	return payload["https://hasura.io/jwt/claims"].(map[string]interface{})["x-hasura-user-id"].(string), nil
}

func returnJsonError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	jsonData, err := json.Marshal(struct {
		Error string `json:"error"`
	}{
		Error: err.Error(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Error(w, string(jsonData), status)
}

func returnJson(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	jsonData, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	_, err = w.Write(jsonData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func main() {

	var errr error
	var platformConfig tools.Config

	platformConfig, errr = tools.LoadConfigFromFile("config.json")

	client, errr = ApiInterface.NewClient(platformConfig.Domain)
	if errr != nil {
		log.Fatal(errr)
	}
	//fmt.Println(client.Run("query queryUser($idUser: Int!){\n  user (where: {id : {_eq: $idUser}}){\n    login\n  }\n}", map[string]interface{}{"idUser": 1102}))

	// API

	http.HandleFunc("/campus", func(w http.ResponseWriter, r *http.Request) {
		// print the campus information in json format
		campus, err := GetCampus(platformConfig.CampusName)
		if err != nil {
			returnJsonError(w, err, http.StatusInternalServerError)
			return
		}
		returnJson(w, struct {
			Id   int    `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		}{
			Id:   campus.Id,
			Name: campus.Name,
			Type: campus.Type,
		})
	})

	http.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		returnJson(w, "not implemented yet")
	})

	http.HandleFunc("/user/extractId", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "x-token")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// read the x-token header
		token := r.Header.Get("x-token")
		if token == "" {
			returnJsonError(w, errors.New("x-token header is missing"), http.StatusBadRequest)
			return
		}
		// extract the user id from the token
		id, err := ExtractId(token)
		if err != nil {
			returnJsonError(w, err, http.StatusBadRequest)
			return
		}
		returnJson(w, struct {
			Id string `json:"id"`
		}{
			Id: id,
		})
	})

	http.HandleFunc("/user/courses", func(w http.ResponseWriter, r *http.Request) {

	})

	http.HandleFunc("/campus/courses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		campus, err := GetCampus(platformConfig.CampusName)
		if err != nil {
			returnJsonError(w, err, http.StatusInternalServerError)
			return
		}
		//return the campus children data in json format
		returnJson(w, campus.Children)
	})

	http.HandleFunc("/campus/register", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, x-token")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// read the x-token header
		token := r.Header.Get("x-token")
		if token == "" {
			returnJsonError(w, errors.New("x-token header is missing"), http.StatusBadRequest)
			return
		}
		// extract the user id from the token
		id, err := ExtractId(token)
		if err != nil {
			returnJsonError(w, err, http.StatusBadRequest)
			return
		}
		// get the course id from the request body
		var body struct {
			CourseId int `json:"courseId"`
		}
		err = json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			returnJsonError(w, err, http.StatusBadRequest)
			return
		}
		// register the user to the course
		//_, err = client.Run("mutation insertUserCourse($userId: Int!, $courseId: Int!){\n  insert_user_course_one(object: {user_id: $userId, course_id: $courseId}) {\n    id\n  }\n}", map[string]interface{}{"userId": id, "courseId": body.CourseId})
		id = id
		fmt.Println(id, body.CourseId)
		if err != nil {
			returnJsonError(w, err, http.StatusInternalServerError)
			return
		}
		returnJson(w, struct {
			Message string `json:"message"`
		}{
			Message: "User registered to the course",
		})
	})

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		return
	}
}
