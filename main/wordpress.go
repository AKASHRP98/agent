package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/sethvargo/go-password/password"
)

func wpAdd(c echo.Context) error {
	// Bind received post request body to a struct
	wp := new(wpadd)
	c.Bind(&wp)

	// Check if all fields are defind
	if wp.AppName == "" || wp.Url == "" || wp.UserName == "" || wp.Title == "" || wp.AdminEmail == "" || wp.AdminPassword == "" || wp.AdminUser == "" {
		result := &errcode{
			Code:    101,
			Message: "Required fields are not defined",
		}
		return c.JSON(http.StatusBadRequest, result)
	}

	// check if user exists or not. If not then create a user with home directory
	_, err := user.Lookup(wp.UserName)
	if err != nil {
		exec.Command("/bin/bash", "-c", fmt.Sprintf("useradd --shell /bin/bash --create-home %s", wp.UserName)).Output()
	}

	// Assign path of home directory to a variable
	path := fmt.Sprintf("/home/%s/", wp.UserName)

	// check if path exists. If not then create a directory
	if _, err := os.Stat(path); os.IsNotExist(err) {
		exec.Command("/bin/bash", "-c", fmt.Sprintf("mkdir %s", path)).Output()
	}

	// Check for appName if already exists or not. If exists send error message
	lsByte, _ := exec.Command("/bin/bash", "-c", fmt.Sprintf("ls %s", path)).Output()
	lsStirng := string(lsByte)
	lsSlice := strings.Split(lsStirng, "\n")

	for _, ls := range lsSlice {
		if ls == wp.AppName {
			result := &errcode{
				Code:    102,
				Message: "App Name exists",
			}
			return c.JSON(http.StatusBadRequest, result)
		}
	}

	// Create random number to concate with appName for prevention of Duplicity. Create rand password for DB password and assign them to DB struct
	randInt, _ := password.Generate(5, 5, 0, false, true)
	pass, _ := password.Generate(32, 20, 0, false, true)
	dbCred := db{fmt.Sprintf("%s_%s", wp.AppName, randInt), fmt.Sprintf("%s_%s", wp.AppName, randInt), pass}

	err = createDatabase(dbCred)
	if err != nil {
		result := &errcode{
			Code:    103,
			Message: "Cannot create database",
		}
		return c.JSON(http.StatusBadRequest, result)
	}

	//Create folder in user home directory for wordpress
	path = fmt.Sprintf("/home/%s/%s", wp.UserName, wp.AppName)
	exec.Command("/bin/bash", "-c", fmt.Sprintf("mkdir %s", path)).Output()
	_, err = exec.Command("/bin/bash", "-c", fmt.Sprintf("chown %s:%s %s", wp.UserName, wp.UserName, path)).Output()
	if err != nil {
		result := &errcode{
			Code:    104,
			Message: "cannot create folder for wordpress",
		}
		return c.JSON(http.StatusBadRequest, result)
	}

	// Download wordpress
	_, err = exec.Command("/bin/bash", "-c", fmt.Sprintf("sudo -u %s -i -- /usr/Hosting/wp-cli core download --path=%s", wp.UserName, path)).Output()
	if err != nil {
		write, _ := json.MarshalIndent(dbCred, "", "  ")
		ioutil.WriteFile("/usr/Hosting/error.log", write, 0777)
		result := &errcode{
			Code:    105,
			Message: "Cannot download wordpress",
		}
		return c.JSON(http.StatusBadRequest, result)
	}

	// Create config file with database crediantls for DB struct
	_, err = exec.Command("/bin/bash", "-c", fmt.Sprintf("sudo -u %s -i -- /usr/Hosting/wp-cli config create --path=%s --dbname=%s --dbuser=%s --dbpass=%s", wp.UserName, path, dbCred.Name, dbCred.User, dbCred.Password)).CombinedOutput()
	if err != nil {
		write, _ := json.MarshalIndent(dbCred, "", "  ")
		ioutil.WriteFile("/usr/Hosting/error.log", write, 0777)
		result := &errcode{
			Code:    106,
			Message: "Connot configure wp-config file",
		}
		return c.JSON(http.StatusBadRequest, result)
	}
	// 	f, err := os.OpenFile(fmt.Sprintf("%s/wp-config.php", path), os.O_APPEND|os.O_WRONLY, 0644)
	// 	if err != nil {
	// 		return echo.NewHTTPError(http.StatusBadRequest, "cannot add http block to wpconfig file")
	// 	}

	// 	f.WriteString(`
	// /*######################################################################
	// ######################################################################
	// ###        DO NOT REMOVE THIS BLOCK. ADDED BY HOSTING            #####*/
	// if ($_SERVER['HTTP_X_FORWARDED_PROTO'] === 'https') {
	// 	$_SERVER['HTTPS'] = 'on';
	// }
	// /*######################################################################*/`)

	// 	f.Close()
	exec.Command("/bin/bash", "-c", fmt.Sprintf("touch %s/.htaccess", path)).Output()
	exec.Command("/bin/bash", "-c", fmt.Sprintf("echo \" %s/.htaccess IN_MODIFY /usr/sbin/service lsws restart\" >> /etc/incron.d/sites.txt", path)).Output()
	exec.Command("/bin/bash", "-c", "incrontab /etc/incron.d/sites.txt").Output()
	exec.Command("/bin/bash", "-c", fmt.Sprintf("chown %s:%s %s/.htaccess", wp.UserName, wp.UserName, path)).Output()

	//Add phpini file
	exec.Command("/bin/bash", "-c", fmt.Sprintf("mkdir -p /usr/local/lsws/php-ini/%s", wp.AppName))
	phpfile, err := os.OpenFile(fmt.Sprintf("/usr/local/lsws/php-ini/%s/php.ini", wp.AppName), os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	phpfile.Write([]byte(`
	[PHP]
	max_execution_time=200
	max_file_uploads=20
	max_input_time=60
	max_input_vars=2000
	memory_limit=256M
	post_max_size=512M
	session.cookie_lifetime=0
	session.gc_maxlifetime=1440
	upload_max_filesize=512M
	`))
	phpfile.Close()
	// Install wordpress with data provided by request
	_, err = exec.Command("/bin/bash", "-c", fmt.Sprintf("sudo -u %s -i -- /usr/Hosting/wp-cli core install --path=%s --url=%s --title=%s --admin_user=%s --admin_password=%s --admin_email=%s", wp.UserName, path, wp.Url, wp.Title, wp.AdminUser, wp.AdminPassword, wp.AdminEmail)).CombinedOutput()

	if err != nil {
		write, _ := json.MarshalIndent(dbCred, "", "  ")
		ioutil.WriteFile("/usr/Hosting/error.log", write, 0777)
		result := &errcode{
			Code:    107,
			Message: "Cannot install wordpress",
		}
		return c.JSON(http.StatusBadRequest, result)
	}
	err = editLsws(*wp)
	if err != nil {
		result := &errcode{
			Code:    108,
			Message: "Edit lsws error",
		}
		return c.JSON(http.StatusBadRequest, result)
	}
	exec.Command("/bin/bash", "-c", fmt.Sprint("mkdir -p /var/logs/Hosting/%s", wp.AppName))

	err = addSiteToJSON(*wp, "live")
	if err != nil {
		result := &errcode{
			Code:    109,
			Message: "Error occured while adding site to json",
		}
		return c.JSON(http.StatusBadRequest, result)
	}

	err = configNuster()
	if err != nil {
		result := &errcode{
			Code:    110,
			Message: "Error occured while configuring nuster",
		}
		return c.JSON(http.StatusBadRequest, result)
	}
	exec.Command("/bin/bash", "-c", "service hosting restart").Output()
	exec.Command("/bin/bash", "-c", fmt.Sprintf("mkdir -p /var/log/hosting/%s", wp.AppName)).Output()
	return c.JSON(http.StatusOK, dbCred)

}

func wpDelete(c echo.Context) error {
	wp := new(wpdelete)
	c.Bind(&wp)
	path := fmt.Sprintf("/home/%s/%s", wp.UserName, wp.AppName)
	exec.Command("/bin/bash", "-c", fmt.Sprintf("rm -rf %s", path)).Output()
	exec.Command("/bin/bash", "-c", fmt.Sprintf("mysql -e \"DROP DATABASE %s;\"", wp.DbName)).Output()
	exec.Command("/bin/bash", "-c", fmt.Sprintf("mysql -e \"DROP USER '%s'@'localhost';\"", wp.DbUser)).Output()
	exec.Command("/bin/bash", "-c", fmt.Sprintf("rm /usr/local/lsws/conf/vhosts/%s.conf", wp.AppName)).Output()
	exec.Command("/bin/bash", "-c", fmt.Sprintf("rm -rf /usr/local/lsws/conf/vhosts/%s.d", wp.AppName)).Output()
	go exec.Command("/bin/bash", "-c", "killall lsphp").Output()
	go exec.Command("/bin/bash", "-c", "service lsws restart").Output()

	err := deleteSiteFromJSON(*wp)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Cannot delete from Json file")
	}

	err = configNuster()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Cannot config nuster file")
	}
	exec.Command("/bin/bash", "-c", fmt.Sprintf("sed -i '/%s\\/%s/d' /etc/incron.d/sites.txt", wp.UserName, wp.AppName)).Output()
	go exec.Command("/bin/bash", "-c", "service hosting restart").Output()

	return c.String(http.StatusOK, "Delete success")
}

func createDatabase(d db) error {
	out, err := exec.Command("/bin/bash", "-c", fmt.Sprintf("mysql -e \"CREATE DATABASE %s;\"", d.Name)).CombinedOutput()
	if err != nil {
		write, _ := json.MarshalIndent(d, "", "  ")
		ioutil.WriteFile("/usr/Hosting/error.log", write, 0777)
		return echo.NewHTTPError(http.StatusBadRequest, string(out))
	}
	out, err = exec.Command("/bin/bash", "-c", fmt.Sprintf("mysql -e \"CREATE USER '%s'@'localhost' IDENTIFIED BY '%s';\"", d.User, d.Password)).CombinedOutput()
	if err != nil {
		write, _ := json.MarshalIndent(d, "", "  ")
		ioutil.WriteFile("/usr/Hosting/error.log", write, 0777)
		return echo.NewHTTPError(http.StatusBadRequest, string(out))
	}
	exec.Command("/bin/bash", "-c", fmt.Sprintf("mysql -e \"GRANT ALL PRIVILEGES ON %s.* TO '%s'@'localhost';\"", d.Name, d.User)).CombinedOutput()
	if err != nil {
		write, _ := json.MarshalIndent(d, "", "  ")
		ioutil.WriteFile("/usr/Hosting/error.log", write, 0777)
		return echo.NewHTTPError(http.StatusBadRequest, err)
	}
	exec.Command("/bin/bash", "-c", "mysql -e 'FLUSH PRIVILEGES;'").Output()

	return nil
}
