package main

/* ==============================================
Copyright (c) Eensymachines
Developed by 		: kneerunjun@gmail.com
Developed on 		: OCT'22
All the middleware tests here
============================================== */
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	testBaseUrl      = "http://localhost"
	REQ_CONTENT_TYPE = "application/json"
)

/*-----------------------------
Helper functions, private utility functions
-----------------------------*/
// getTestMongoColl : will get a mongo collection using the test connection
// tests are run on host machines and hence the mongo access is using the localhost connection
// mongo runs from within the container and hence the port that the container gets shared is importatn
func getTestMongoColl(name, dbname string) *mongo.Collection {
	todo, _ := context.WithTimeout(context.Background(), 10*time.Second)
	client, err := mongo.Connect(todo, options.Client().ApplyURI("mongodb://localhost:37017"))
	if err != nil {
		log.Errorf("getTestMongoColl: failed to connect to database %s", err)
	}
	if client.Ping(todo, nil) != nil {
		log.Error("getTestMongoColl: not connected to database, ping failed")
	}
	return client.Database(dbname).Collection(name)
}

// newTestGinContext : since we are testing on the middleware level we need to make a new mock context
func newTestGinContext(body interface{}, method, path string) *gin.Context {
	byt, _ := json.Marshal(body)
	reader := bytes.NewReader(byt)
	return &gin.Context{
		Request: &http.Request{
			Method: method,
			URL:    &url.URL{Path: fmt.Sprintf("http://sample.com/mockrequest/%s", path)},
			Proto:  "HTTP/1.1",
			Header: map[string][]string{
				"Accept-Encoding": {"application/json"},
			},
			Body: io.NopCloser(reader),
		},
	}
}
func TestAccountPayload(t *testing.T) {
	// Wil test the unmarshalling of the account into the route payload
	// https://gosamples.dev/struct-to-io-reader/
	byt, _ := json.Marshal(&UserAccount{})
	reader := bytes.NewReader(byt)
	ctx := &gin.Context{
		Request: &http.Request{
			Method: "POST",
			URL:    &url.URL{Path: "http://sample.com/applications/accounts"},
			Proto:  "HTTP/1.1",
			Header: map[string][]string{
				"Accept-Encoding": {"application/json"},
			},
			Body: io.NopCloser(reader),
		},
	}
	AccountPayload(ctx)
	acc, _ := ctx.Get("account")
	assert.NotNil(t, acc, "unpected fail to set accout object in context")
	// Setting an account with values
	sampleAcc := &UserAccount{
		Email: "john.dore@gmail.com",
		Phone: "8980982093",
		Title: "John Dore",
	}
	byt, _ = json.Marshal(sampleAcc)
	reader = bytes.NewReader(byt)
	ctx.Request.Body = io.NopCloser(reader)
	AccountPayload(ctx)
	val, _ := ctx.Get("account")
	accVal, _ := val.(Account)
	assert.NotNil(t, acc, "Unexpected nil value on the account")
	// Then checking for the value to determine if the values we sent in are the same that come out
	assert.Equal(t, accVal.GetEmail(), sampleAcc.Email, "could not match verify the email")
	assert.Equal(t, accVal.GetTitle(), sampleAcc.Title, "could not match verify the title")
	assert.Equal(t, accVal.GetPhone(), sampleAcc.Phone, "could not match verify the phone")

	// Setting a nil account in context
	byt, _ = json.Marshal(nil)
	reader = bytes.NewReader(byt)
	ctx.Request.Body = io.NopCloser(reader)
	AccountPayload(ctx)
	acc, _ = ctx.Get("account")
	assert.NotNil(t, acc, "unexpected nil account has been set in context")
}

// TestInsertOne : wrote this test when the object id being inserted was having problems with _id
// Desired result is while marshalling we want a new _id being generated by mongo
// while unmarshaling we want the _id to be read back
func TestInsertOne(t *testing.T) {
	// ======== setting up the test
	// ======== database connection
	ua, err := JsonSampleRandomAccount()
	if err != nil {
		t.Error("TestInsertOne: failed to get random sample account")
	}
	t.Log(ua)
	coll := getTestMongoColl("accounts", "eensydb")
	assert.NotNil(t, coll, "Unexpected null collection, cannot proceed with test")
	result, err := coll.InsertOne(context.TODO(), ua)
	// =================
	// ========= Testing
	assert.Nil(t, err, "failed to insert simple account")
	assert.NotNil(t, result.InsertedID, "unexpected nil id from insertion")
	t.Logf("Inserted id %v", result.InsertedID)
	// ========= Getting the inserted document
	sr := coll.FindOne(context.TODO(), bson.M{
		"_id": result.InsertedID,
	})
	assert.Nil(t, sr.Err(), "unexpected error when getting the user account")
	usrAcc := UserAccount{}
	err = sr.Decode(&usrAcc)
	assert.Nil(t, err, "Unexpected error when decoding user account")
	assert.Equal(t, ua.Email, usrAcc.Email, "Email of the accounts do not match")
	assert.Equal(t, ua.Title, usrAcc.Title, "Email of the accounts do not match")
	// And then when you are done inserting you can delete the account
	// ========= Cleaning up
	coll.DeleteOne(context.TODO(), bson.M{
		"email": ua.Email,
	})
}

// TestAccPostMddlware : this will test posting a new account to the database via middleware
// Mocking up the http request and then pushing it in
// TEsting the middleware isnt a good idea since setting up the test itsself takes a lot of code
// plus debug has seen panics when the middleware uses c.AbortWithStatus()
// Not that there isnt a way to do this, but there certainly is a easier way to do this
// that being - testing the API endpoint and not the middleware
func TestAccPostMddlware(t *testing.T) {
	// ======= setting up the test
	ua, err := JsonSampleRandomAccount()
	if err != nil {
		t.Error("TestInsertOne: failed to get random sample account")
	}
	t.Log(ua)
	ctx := newTestGinContext(ua, "POST", "accounts")

	// We woudl have to make a database connection to have it injected in the context
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:37017"))
	if err != nil {
		log.Panicf("failed to connect to database %s", err)
	}
	collAccs := client.Database("eensydb").Collection("accounts")
	ctx.Set("coll", collAccs)
	// Test
	AccountPayload(ctx)
	Accounts(ctx)
	assert.Equal(t, ctx.Request.Response.StatusCode, http.StatusCreated, "http status code not as expected")
	// ======= Cleaning up the test
}

// TestApiPostAcc : this is to get the account tested from the api end point
func TestApiPostAcc(t *testing.T) {
	/* ==========
	Setting up the test
	=============*/
	coll := getTestMongoColl("accounts", "eensydb") // database connection from host port
	assert.NotNil(t, coll, "Unexpected null collection, cannot proceed with test")
	ua, err := JsonSampleRandomAccount() // sample account to be inserted from json seed
	if err != nil {
		t.Error("TestInsertOne: failed to get random sample account")
	}
	t.Logf("Database connection: ok, sample account to post %s", ua.Email)
	/* ==========
	defered Cleaning up the test
	=============*/
	defer func() {
		t.Log("Now cleaning up the test.. ")
		coll.DeleteOne(context.TODO(), bson.M{
			"email": ua.Email,
		})
		coll.Database().Client().Disconnect(context.TODO())
	}()
	/* ==========
	POST API HTTP test
	=============*/
	testUrl := fmt.Sprintf("%s/api/accounts", testBaseUrl)
	bytJson, err := json.Marshal(ua)
	if err != nil {
		t.Errorf("failed to marshall account to json")
	}
	payload := bytes.NewReader(bytJson)
	t.Logf("now testing POST with url %s", testUrl)
	resp, err := http.Post(testUrl, REQ_CONTENT_TYPE, payload)
	if err != nil {
		t.Errorf("error making the POST request, %s", err)
	}
	defer resp.Body.Close()
	assert.Equal(t, 201, resp.StatusCode, fmt.Sprintf("Unexpected status code for request %s", testUrl))
	/* ==========
	Checking for new account being added to the database
	=============*/
	// TODO: the api shoudl get back the details of the account being inserted
	//
	count, err := coll.CountDocuments(context.TODO(), bson.M{
		"email": ua.Email,
	})
	if err != nil {
		t.Errorf("failed database query to read account being added %s", err)
	}
	assert.Equal(t, int64(1), count, "Unexp3cted count of documents when inserted")
	/* ==========
	Reading the response payload
	=============*/
	byt, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("error reading the response body, %s", err)
	}
	result := map[string]interface{}{}
	json.Unmarshal(byt, &result)
	assert.NotNil(t, result, "Unexpected nil response payload")
	t.Log(result)
	/* ==========
	GET API HTTP test
	=============*/
	// Then trying to get the account details from api
	// remember result["id"] is a string fromt the json payload
	testUrl = fmt.Sprintf("%s/api/accounts/%s", testBaseUrl, result["id"])
	t.Logf("now testing GET with url %s", testUrl)
	resp, err = http.Get(testUrl)
	if err != nil {
		t.Errorf("error making the GET request, %s", err)
	}
	defer resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode, fmt.Sprintf("Unexpected status code for request %s", testUrl))
	byt, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("error reading the response body, %s", err)
	}
	accResult := UserAccount{}
	json.Unmarshal(byt, &accResult)
	assert.NotNil(t, result, "Unexpected nil response payload")
	t.Log(accResult)
	/* ==========
	GET test with invalid id
	=============*/
	invalidID := primitive.NewObjectID()
	testUrl = fmt.Sprintf("%s/api/accounts/%s", testBaseUrl, invalidID.Hex())
	t.Logf("now testing GET with account id that does not exists %s", testUrl)
	resp, err = http.Get(testUrl)
	if err != nil {
		t.Errorf("Error GET request, invalid id, %s", err)
	}
	assert.Equal(t, 404, resp.StatusCode, fmt.Sprintf("Unexpected status code for request %s", testUrl))
	/* ==========
	PUT test with valid payload
	=============*/
	testUrl = fmt.Sprintf("%s/api/accounts/%s", testBaseUrl, result["id"])
	t.Logf("now testing PUT with url %s", testUrl)
	bytJson, _ = json.Marshal(&UserAccount{
		Title: "testPutTitle",
		Phone: "8390906860",
	})
	reader := bytes.NewReader(bytJson)
	req, err := http.NewRequest("PUT", testUrl, reader)
	if err != nil {
		t.Error(err)
	}
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Errorf("Error GET request, invalid id, %s", err)
	}
	assert.Equal(t, 200, resp.StatusCode, fmt.Sprintf("Unexpected status code for request %s", testUrl))
}
