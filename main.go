package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	color "kubelogns/colors"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lithammer/fuzzysearch/fuzzy"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const VERSION string = "1.0.3"
const AUTHOR string = "https://github.com/itspacchu"

var (
	fupServer       string
	didIWarnAlready bool = false
)

func getPodLogs(pod corev1.Pod, clientset *kubernetes.Clientset, stdout bool) string {
	podLogOpts := corev1.PodLogOptions{}
	req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)
	podLogs := req.Do(context.Background())
	rawPodLogs, err := podLogs.Raw()
	if err != nil {
		return "Unable to get pod logs"
	}
	if stdout {
		podLogs := string(rawPodLogs)
		for _, m := range strings.Split(podLogs, "\n") {
			if len(m) < 1 {
				continue
			}
			fmt.Printf(color.Purple+"[%s]"+color.Reset+": %s\n", pod.Name, m)
		}
	}
	return string(rawPodLogs)
}

func uploadFile(filepath string, expires int) (string, error) {
	file, err := os.Open(filepath)
	url := fupServer
	if url == "" {
		if !didIWarnAlready {
			fmt.Printf(color.Yellow + "WARN" + color.Reset + ": No FUP server provided using default https://0x0.st (This UPLOADS Everything!!!) \n")
			didIWarnAlready = true
		}

		url = "https://0x0.st"
	}
	if err != nil {
		return "", err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return "", err
	}

	err = writer.WriteField("expires", fmt.Sprintf("%d", expires))
	if err != nil {
		return "", err
	}
	writer.Close()

	request, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	var result bytes.Buffer
	_, err = io.Copy(&result, response.Body)
	if err != nil {
		return "", err
	}

	return result.String(), nil
}

func main() {
	var printVersion bool
	var kubeconfig *string
	var namespace string
	var filename string
	var upload bool
	var fuzzymode string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.StringVar(&namespace, "n", "", "Namespace to fetch pod logs from")
	flag.StringVar(&filename, "o", "", "File to output logs to")
	flag.StringVar(&filename, "s", "", "FUP server to use (defaults to https://0x0.st)")
	flag.BoolVar(&upload, "u", false, "Upload to fup server")
	flag.BoolVar(&printVersion, "v", false, "Version info printing")
	flag.StringVar(&fuzzymode, "f", "", "Fuzzy Search pod names in namespace")
	flag.Parse()

	if printVersion {
		fmt.Printf("Version %s\nAuthor: %s\n", VERSION, AUTHOR)
		return
	}

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if namespace == "" {
		namespace, _, _ = clientcmd.DefaultClientConfig.Namespace()
	}
	if err != nil {
		panic(err.Error())
	}
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	if !upload {
		fmt.Printf(color.Green+"INFO"+color.Reset+": Fetching Pod logs for %s namespace :\n", namespace)
	} else {
		fmt.Printf(color.Green+"INFO"+color.Reset+": Writing Pod logs for %s namespace to %s:\n", namespace, filename)
	}
	for _, pod := range pods.Items {
		if !fuzzy.Match(fuzzymode, pod.Name) && fuzzymode != "" {
			continue
		}
		if filename == "" && !upload {
			getPodLogs(pod, clientset, true)
			fmt.Println("---")
		} else {
			if upload && filename == "" {
				filename = "tmp"
			}
			if filename != "" {
				foldername := strings.Split(filename, ".")[0]
				podpath := foldername + "/" + pod.Namespace + "_" + filename
				os.Mkdir(foldername, 0755)
				f, err := os.Create(podpath)
				if err != nil {
					fmt.Printf(color.Red+"ERR"+color.Reset+": Error writing logs to %s. %s\n", podpath, err)
				}
				defer f.Close()
				os.WriteFile(podpath, []byte(getPodLogs(pod, clientset, false)), 0644)
				if upload {
					upload_url, _ := uploadFile(podpath, 1)
					fmt.Println(color.Blue + fupServer + " - " + color.Purple + "( " + pod.Name + " )" + color.Reset + ": " + upload_url)
					if filename == "" {
						os.Remove(podpath)
					}
					time.Sleep(1 * time.Second)
				}
			}
		}
	}

}
