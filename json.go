package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/labstack/echo/v4"
)

func EditNuster() error {

	// read file
	data, err := ioutil.ReadFile("/usr/Hosting/config.json")
	if err != nil {
		return echo.NewHTTPError(404, "Config file not found")
	}

	// define data structure
	type Global struct {
		Datasize int `json:"dataSize"`
		Maxconn  int `json:"maxConnection"`
	}
	type Timeout struct {
		Connect int `json:"connect"`
		Client  int `json:"client"`
		Server  int `json:"server"`
	}

	type Default struct {
		Timeout Timeout `json:"timeout"`
	}

	type Site struct {
		Name     string   `json:"name"`
		Domain   []string `json:"domain"`
		SSL      int      `json:"ssl"`
		Cache    string   `json:"cache"`
		Redirect bool     `json:"redirect"`
	}

	type Config struct {
		Global  Global  `json:"global"`
		Default Default `json:"defaults"`
		Sites   []Site  `json:"sites"`
	}

	// json data
	var obj Config

	// unmarshall it
	err = json.Unmarshal(data, &obj)
	if err != nil {
		return echo.NewHTTPError(400, "JSON data error")
	}

	conf := "##############################\n# Do not edit this file#\n#############################\n"

	conf = conf + fmt.Sprintf("global\n\tnuster cache on data-size %dm\n\tmaster-worker\n\tmaxconn %d\n", obj.Global.Datasize, obj.Global.Maxconn)

	conf = conf + fmt.Sprintf("defaults\n\tmode http\n\ttimeout connect %ds\n\ttimeout client %ds\n\ttimeout server %ds\n", obj.Default.Timeout.Connect, obj.Default.Timeout.Client, obj.Default.Timeout.Server)

	conf = conf + `frontend nonssl
    bind *:80`
	for _, frontend := range obj.Sites {
		conf = conf + fmt.Sprintf(`
	acl host_%s hdr(host) -i %s`, frontend.Name, strings.Trim(fmt.Sprint(frontend.Domain), "[]"))
	}

	conf = conf + `
    acl has_cookie hdr_sub(cookie) wordpress_logged_in
    acl has_path path_sub wp-admin || wp-login
    acl static_file path_end .js || .css || .png || .jpg || .jpeg || .gif || .ico
    use_backend nocache if has_path || has_cookie
    use_backend static if static_file`

	for _, frontend := range obj.Sites {
		conf = conf + fmt.Sprintf(`
	use_backend %s if host_%s`, frontend.Name, frontend.Name)
	}

	for _, backend := range obj.Sites {

		conf = conf + fmt.Sprintf(`
backend %s
    nuster cache %s
    nuster rule 200
    http-response set-header x-cache HIT if { nuster.cache.hit }
    http-response set-header x-cache MISS unless { nuster.cache.hit }
    server s1 0.0.0.0:8088`, backend.Name, backend.Cache)
	}
	conf = conf + `
backend nocache
    http-response set-header x-cache BYPASS
    server s2 0.0.0.0:8088
backend static
    http-response set-header x-type STATIC
    server s2 0.0.0.0:8088`
	// the WriteFile method returns an error if unsuccessful
	err = ioutil.WriteFile("/opt/hosting.cfg", []byte(conf), 0777)
	// handle this error
	if err != nil {
		// print it out
		return echo.NewHTTPError(404, err)
	}

	return nil

}
