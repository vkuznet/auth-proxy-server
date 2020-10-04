package main

// https://gist.github.com/josemarjobs/23acc123b3cce1b251a5d5bafdca1140
// https://www.thepolyglotdeveloper.com/2017/03/authenticate-a-golang-api-with-json-web-tokens/
// https://github.com/dgrijalva/jwt-go
// https://godoc.org/github.com/dgrijalva/jwt-go#example-NewWithClaims--CustomClaimsType
// https://demo.scitokens.org/
// https://scitokens.org/
// https://github.com/scitokens/x509-scitokens-issuer/blob/master/tools/cms-scitoken-init.go
// https://github.com/scitokens/scitokens

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	// jwt "github.com/cristalhq/jwt/v3"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
)

// SciTokensSconfig represents configuration of scitokens service
type ScitokensConfig struct {
	FileGlog  string `json:"CONFIG_FILE_GLOB"`
	Lifetime  int    `json:"LIFETIME"`
	IssuerKey string `json:"ISSUER_KEY"`
	Rules     string `json:"RULES"`
	DNMapping string `json:"DN_MAPPING"`
	CMS       bool   `json:"CMS"`
	Verbose   bool   `json:"VERBOSE"`
	Enabled   bool   `json:"ENABLED"`
	Secret    string `json:"SECRET"`
}

var scitokensConfig ScitokensConfig

// TokenResponse rerpresents structure of returned scitoken
type TokenResponse struct {
	AccessToken string `json:"access_token"` // access token string
	TokenType   string `json:"token_type"`   // token type string
	Expires     int64  `json:"expires_in"`   // token expiration
}

// helper function to handle http server errors
func handleError(w http.ResponseWriter, r *http.Request, rec map[string]string) {
	log.Println(Stack())
	log.Printf("error %+v\n", rec)
	data, err := json.Marshal(rec)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("unable to marshal data, error=%v", err)))
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Write(data)
}

// helper function to generate UUID
func genUUID() string {
	uuidWithHyphen := uuid.New()
	uuid := strings.Replace(uuidWithHyphen.String(), "-", "", -1)
	return uuid
}

// helper function to generate scopes and user fields
func generateScopesUser(entries []string) ([]string, string) {
	var scopes []string
	var user string
	return scopes, user
}

// scitokensHandler handle requests for x509 clients
func scitokensHandler(w http.ResponseWriter, r *http.Request) {
	errRecord := make(map[string]string)
	err := r.ParseForm()
	if err != nil {
		log.Printf("could not parse http form, error %v\n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	grantType := r.FormValue("grant_type")
	if grantType != "client_credentials" {
		errRecord["error"] = fmt.Sprintf("Incorrect grant_type %s", grantType)
		handleError(w, r, errRecord)
		return
	}
	// get scopes, should in a form of "write:/store/user/username read:/store"
	var scopes []string
	for _, s := range strings.Split(r.FormValue("scopes"), " ") {
		scopes = append(scopes, strings.Trim(s, " "))
	}
	// defaults
	//     creds := make(map[string]string)
	// TODO: fill out creds with dn, username, fqan obtained from x509 call

	userData := getUserData(r)
	if Config.Verbose > 0 {
		log.Printf("user data %+v\n", userData)
	}
	var sub string
	if v, ok := userData["name"]; ok {
		sub = v.(string)
	} else {
		errRecord["error"] = fmt.Sprintf("No CMS credentials found in TLS authentication")
		handleError(w, r, errRecord)
		return
	}
	// Compare the generated scopes against the requested scopes (if given)
	// If we don't give the user everything they want, then we
	// TODO: parse roles and creat scopes
	if roles, ok := userData["roles"]; ok {
		rmap := roles.(map[string][]string)
		for k, _ := range rmap {
			scopes = append(scopes, k)
		}
	} else {
		errRecord["error"] = "No applicable roles found"
		handleError(w, r, errRecord)
		return
	}

	if len(scopes) == 0 {
		errRecord["error"] = "No applicable scopes for this user"
		handleError(w, r, errRecord)
		return
	}
	// issuer should be hostname of our server
	var issuer string
	if v, ok := userData["issuer"]; ok {
		issuer = v.(string)
	}
	// jti is JWT ID
	jti := genUUID()
	// generate new token and return it back to user
	expires := time.Now().Add(time.Minute * time.Duration(scitokensConfig.Lifetime)).Unix()
	token, err := getSciToken(issuer, jti, sub, strings.Join(scopes, " "))
	if err != nil {
		w.Write([]byte(fmt.Sprintf("unable to get token, error=%v", err)))
		return
	}
	resp := TokenResponse{AccessToken: token, TokenType: "bearer", Expires: expires}
	data, err := json.Marshal(resp)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("unable to marshal data, error=%v", err)))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// ScitokensClaims represent structure of scitokens claims
type ScitokensClaims struct {
	Scope string `json:"scope"` // user's scopes
	jwt.StandardClaims
}

// helper function to generate RSA key
func getRSAKey(fname string) (*rsa.PrivateKey, error) {
	if fname == "" {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		return key, err
	}
	file, err := os.Open("/Users/vk/.ssh/id_rsa")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	key, err := rsa.GenerateKey(file, 2048)
	return key, err
}

/*
// helper function to get scitoken, it is based on
// github.com/cristalhq/jwt/v3
func getSciToken(issuer, jti, sub, scopes string) (string, error) {
	// Create a new token object, specifying signing method and the claims
	expires := jwt.NewNumericDate(time.Now().Add(time.Minute * time.Duration(scitokensConfig.Lifetime)))
	now := jwt.NewNumericDate(time.Now())
	iat := jwt.NewNumericDate(time.Now())
	// for definitions see
	// https://godoc.org/github.com/dgrijalva/jwt-go#StandardClaims
	// https://tools.ietf.org/html/rfc7519#section-4.1
	claims := ScitokensClaims{
		scopes,
		jwt.StandardClaims{
			ExpiresAt: expires, // exp
			Issuer:    issuer,  // iss
			IssuedAt:  iat,     // iat
			ID:        jti,     // jti
			Subject:   sub,     // sub
			NotBefore: now,     // nbf
		},
	}
	//     fname := "/Users/vk/.ssh/id_rsa"
	fname := ""
	key, err := getRSAKey(fname)
	signer, err := jwt.NewSignerRS(jwt.RS256, key)
	if err != nil {
		log.Fatal(err)
	}
	builder := jwt.NewBuilder(signer)
	token, err := builder.Build(claims)
	tokenString := token.String()
	return tokenString, err
}
*/

// helper function to get scitoken
func getSciToken(issuer, jti, sub, scopes string) (string, error) {
	// Create a new token object, specifying signing method and the claims
	expires := time.Now().Add(time.Minute * time.Duration(scitokensConfig.Lifetime)).Unix()
	now := time.Now().Unix()
	iat := now
	// for definitions see
	// https://godoc.org/github.com/dgrijalva/jwt-go#StandardClaims
	// https://tools.ietf.org/html/rfc7519#section-4.1
	claims := ScitokensClaims{
		scopes,
		jwt.StandardClaims{
			ExpiresAt: expires, // exp
			Issuer:    issuer,  // iss
			IssuedAt:  iat,     // iat
			Id:        jti,     // jti
			Subject:   sub,     // sub
			NotBefore: now,     // nbf
		},
	}
	//     fname := "/Users/vk/.ssh/id_rsa"
	fname := ""
	key, err := getRSAKey(fname)

	//     token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	//     secret := []byte(scitokensConfig.Secret)
	//     tokenString, err := token.SignedString(secret)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(key)
	return tokenString, err
}

// helper function to start scitokens server
func scitokensServer() {
	// check if provided crt/key files exists
	serverCrt := checkFile(Config.ServerCrt)
	serverKey := checkFile(Config.ServerKey)

	// the server settings handler
	base := Config.Base
	http.HandleFunc(fmt.Sprintf("%s/server", base), settingsHandler)
	// metrics handler
	http.HandleFunc(fmt.Sprintf("%s/metrics", base), metricsHandler)
	// static content
	http.Handle(fmt.Sprintf("%s/.well-known/", base), http.StripPrefix(base+"/.well-known/", http.FileServer(http.Dir(Config.WellKnown))))

	// the request handler
	http.HandleFunc(fmt.Sprintf("%s/token", base), scitokensHandler)

	// start HTTPS server
	server, err := getServer(serverCrt, serverKey, true)
	if err != nil {
		log.Fatalf("unable to start scitokens server, error %v\n", err)
	}
	log.Fatal(server.ListenAndServeTLS(serverCrt, serverKey))
}