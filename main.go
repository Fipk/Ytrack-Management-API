package main

import (
	"Ytrack-Manager/ApiInterface"
	"Ytrack-Manager/tools"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
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

type Course struct {
	Id     int    `json:"id"`
	Name   string `json:"name"`
	Campus string `json:"campus"`
}

func GetCampusCourses(campusName string, client *ApiInterface.Client) ([]Course, error) {
	data, err := client.Run("query queryCampusEvents($campusName : String!){\n  event (where: {_and: [{campus: {_eq: $campusName}}, {object: {type: {_eq: \"piscine\"}}}]}){\n    id\n    object{\n      campus\n      name\n    }\n  }\n}", map[string]interface{}{"campusName": campusName})
	if err != nil {
		return nil, err
	}
	var courses []Course
	for _, v := range data["event"].([]interface{}) {
		course := v.(map[string]interface{})
		courses = append(courses, Course{
			Id:     int(course["id"].(float64)),
			Name:   course["object"].(map[string]interface{})["name"].(string),
			Campus: course["object"].(map[string]interface{})["campus"].(string),
		})
	}

	return courses, nil
}

func GetUserCourses(campusName string, userId int, client *ApiInterface.Client) ([]Course, error) {
	data, err := client.Run("query queryUserEvents($campusName : String!,$userID : Int!){\n  user (where:{id:{_eq: $userID}}){\n    events (where: {_and: [{event:{campus: {_eq: $campusName}}}, {event:{object: {type: {_eq: \"piscine\"}}}}]}){\n      event{\n        id\n        object{\n          campus\n          name\n        }\n      }\n    }\n  }\n}", map[string]interface{}{"campusName": campusName, "userID": userId})
	if err != nil {
		return nil, err
	}
	var courses []Course
	for _, v := range data["user"].([]interface{})[0].(map[string]interface{})["events"].([]interface{}) {
		course := v.(map[string]interface{})["event"].(map[string]interface{})
		courses = append(courses, Course{
			Id:     int(course["id"].(float64)),
			Name:   course["object"].(map[string]interface{})["name"].(string),
			Campus: course["object"].(map[string]interface{})["campus"].(string),
		})
	}

	return courses, nil
}

func RegisterUserToCourse(userId int, courseId int, client *ApiInterface.Client) error {
	_, err := client.Run("mutation insert_event_user ($objects: [event_user_insert_input!]!){\n    insert_event_user (objects: $objects) { returning { eventId } }\n  }", map[string]interface{}{"objects": []map[string]interface{}{{"eventId": courseId, "userId": userId}}})
	if err != nil {
		return err
	}
	return nil
}

func RemoveUserFromCourse(userId int, courseId int, client *ApiInterface.Client) error {
	_, err := client.Run("mutation remove_user_from_event($userId: Int!, $eventId: Int!) {\n    delete_event_user(\n      where: {\n        _and: [{ userId: { _eq: $userId } }, { eventId: { _eq: $eventId } }]\n      }\n    ) {\n      affected_rows\n    }\n  }", map[string]interface{}{"userId": userId, "eventId": courseId})
	if err != nil {
		return err
	}
	return nil
}

func ExtractId(token string) (int, error) {
	payload, err := ApiInterface.Decode(token)
	if err != nil {
		return 0, err
	}
	id, err := strconv.Atoi(payload["https://hasura.io/jwt/claims"].(map[string]interface{})["x-hasura-user-id"].(string))
	return id, nil
}

func ExtractRoles(token string) ([]string, error) {
	payload, err := ApiInterface.Decode(token)
	if err != nil {
		return nil, err
	}
	roles := payload["https://hasura.io/jwt/claims"].(map[string]interface{})["x-hasura-allowed-roles"].([]interface{})
	var rolesString []string
	for _, role := range roles {
		rolesString = append(rolesString, role.(string))
	}
	return rolesString, nil
}

func returnJsonError(w http.ResponseWriter, err error, status int) {
	log.Println(err)
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

	// API

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		returnJson(w, "Welcome to the Ytrack Manager API")
	})

	http.HandleFunc("/swagger/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "swagger/"+r.URL.Path[8:])
	})

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

	http.HandleFunc("/user/name", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", w.Header().Get("Origin"))
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
		// get the user name
		data, err := client.Run("query get_user_name($userID : Int!){\n  user(where:{id:{_eq: $userID}}){\n\t\tfirstName\nlastName\n  }\n}", map[string]interface{}{"userID": id})
		if err != nil {
			returnJsonError(w, err, http.StatusInternalServerError)
			return
		}
		returnJson(w, struct {
			FirstName string `json:"firstName"`
			LastName  string `json:"lastName"`
		}{
			FirstName: data["user"].([]interface{})[0].(map[string]interface{})["firstName"].(string),
			LastName:  data["user"].([]interface{})[0].(map[string]interface{})["lastName"].(string),
		})
	})

	http.HandleFunc("/user/roles", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", w.Header().Get("Origin"))
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
		// extract the user roles from the token
		roles, err := ExtractRoles(token)
		if err != nil {
			returnJsonError(w, err, http.StatusBadRequest)
			return
		}
		returnJson(w, struct {
			Roles []string `json:"roles"`
		}{
			Roles: roles,
		})
	})

	http.HandleFunc("/user/extractId", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", w.Header().Get("Origin"))
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
			Id int `json:"id"`
		}{
			Id: id,
		})
	})

	http.HandleFunc("/user/courses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", w.Header().Get("Origin"))
		w.Header().Set("Access-Control-Allow-Headers", "x-token")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// add a delay to test the loading spinner
		time.Sleep(500 * time.Millisecond)
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
		// get the user courses
		courses, err := GetUserCourses(platformConfig.CampusName, id, client)
		if err != nil {
			returnJsonError(w, err, http.StatusInternalServerError)
			return
		}
		if len(courses) == 0 {
			returnJson(w, []string{})
		} else {
			returnJson(w, courses)
		}
	})

	http.HandleFunc("/user/availableCourses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", w.Header().Get("Origin"))
		w.Header().Set("Access-Control-Allow-Headers", "x-token")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// add a delay to test the loading spinner
		time.Sleep(500 * time.Millisecond)
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
		// get the campus courses
		courses, err := GetCampusCourses(platformConfig.CampusName, client)
		if err != nil {
			returnJsonError(w, err, http.StatusInternalServerError)
			return
		}
		// get the user courses
		userCourses, err := GetUserCourses(platformConfig.CampusName, id, client)
		if err != nil {
			returnJsonError(w, err, http.StatusInternalServerError)
			return
		}
		// filter the campus courses to get the available courses
		var availableCourses []Course
		for _, course := range courses {
			found := false
			for _, userCourse := range userCourses {
				if course.Id == userCourse.Id {
					found = true
					break
				}
			}
			if !found {
				availableCourses = append(availableCourses, course)
			}
		}
		returnJson(w, availableCourses)
	})

	http.HandleFunc("/campus/courses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", w.Header().Get("Origin"))
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		campus, err := GetCampusCourses(platformConfig.CampusName, client)
		if err != nil {
			returnJsonError(w, err, http.StatusInternalServerError)
			return
		}
		returnJson(w, campus)
	})

	http.HandleFunc("/campus/courses/register", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", w.Header().Get("Origin"))
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
		// extract the user userId from the token
		userId, err := ExtractId(token)
		if err != nil {
			returnJsonError(w, err, http.StatusBadRequest)
			return
		}
		// get the course userId from the request body
		var body struct {
			CourseId int `json:"courseId"`
		}
		err = json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			returnJsonError(w, err, http.StatusBadRequest)
			return
		}
		err = RegisterUserToCourse(userId, body.CourseId, client)
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

	http.HandleFunc("/campus/courses/unregister", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", w.Header().Get("Origin"))
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
		// extract the user userId from the token
		userId, err := ExtractId(token)
		if err != nil {
			returnJsonError(w, err, http.StatusBadRequest)
			return
		}
		// get the course userId from the request body
		var body struct {
			CourseId int `json:"courseId"`
		}
		err = json.NewDecoder(r.Body).Decode(&body)
		if err != nil {
			returnJsonError(w, err, http.StatusBadRequest)
			return
		}
		// register the user to the course
		err = RemoveUserFromCourse(userId, body.CourseId, client)
		if err != nil {
			returnJsonError(w, err, http.StatusInternalServerError)
			return
		}
		returnJson(w, struct {
			Message string `json:"message"`
		}{
			Message: "User unregistered from the course",
		})
	})

	// read in the config file if this is a local environment
	if platformConfig.LocalStart == true {
		fmt.Println("Server started on port 8080")
		log.Fatalln(http.ListenAndServe(":8080", nil))
	} else {
		port := os.Getenv("PORT")
		fmt.Println("Server started on port " + port)
		addr := net.JoinHostPort("::", port)
		server := &http.Server{Addr: addr}
		log.Fatalln(server.ListenAndServe())
	}

}
