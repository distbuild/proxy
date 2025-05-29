package consul

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	listenAddresses []string
)

type NormalService struct {
	Name string `json:"ServiceName"`
}

type ConsulService struct {
	Address string      `json:"ServiceAddress"`
	Meta    ServiceMeta `json:"ServiceMeta"`
}

type ServiceMeta struct {
	CreationTime string `json:"CreationTime"`
	CPU          string `json:"cpu"`
	Disks        string `json:"disks"`
	Memory       string `json:"memory"`
}

type Disk struct {
	Name string `json:"name"`
	Size string `json:"size"`
}

func containsAny(listB, listA []string) bool {
	for _, a := range listA {
		if slices.Contains(listB, a) {
			return true
		}
	}
	return false
}

func fetchCompileDiskSize(disksInfo string) (string, error) {
	var disks []Disk
	err := json.Unmarshal([]byte(disksInfo), &disks)
	if err != nil {
		return "", err
	}
	for _, item := range disks {
		if item.Name == "/home" {
			return item.Size, nil
		}
	}
	for _, item := range disks {
		if item.Name == "/" {
			return item.Size, nil
		}
	}
	return "", fmt.Errorf("failed to get compile disk size")
}

func isDiskSizeEnough(disksInfo string) bool {
	homeCapacityInfo, err := fetchCompileDiskSize(disksInfo)
	if err != nil {
		return false
	}
	sizeSlice := strings.Split(homeCapacityInfo, " ")
	if len(sizeSlice) >= 2 {
		sizeUnit := sizeSlice[1]
		switch sizeUnit {
		case "GB":
			num, err := strconv.Atoi(sizeSlice[0])
			if err != nil {
				return false
			}
			if num >= 500 {
				return true
			} else {
				return false
			}
		case "TB":
			return true
		default:
			return false
		}
	}
	return false
}

func isValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

func getListenAddress(consulIp, service string) ([]string, error) {
	url := fmt.Sprintf("http://%s:8500/v1/catalog/service/%s", consulIp, service)
	client := &http.Client{Timeout: 20 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid response status code: %d", resp.StatusCode)
	}

	var services []ConsulService
	err = json.Unmarshal(body, &services)
	if err != nil {
		return nil, err
	}

	var addresses []string
	for _, service := range services {
		// Is diskSizeEnough：available Capacity > 500GB
		diskSizeEnough := isDiskSizeEnough(service.Meta.Disks)

		// if !isValidIP(service.Address) || !dataValid || !diskSizeEnough {
		if !isValidIP(service.Address) || !diskSizeEnough {
			continue
		}
		addresses = append(addresses, fmt.Sprintf("%s:%d", service.Address, 39090))
	}

	return addresses, nil
}

func getNormalConsulServices(consulIp string) ([]string, error) {
	url := fmt.Sprintf("http://%s:8500/v1/health/state/passing", consulIp)
	client := &http.Client{Timeout: 20 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid consul server response status code: %d", resp.StatusCode)
	}

	var services []NormalService
	err = json.Unmarshal(body, &services)
	if err != nil {
		return nil, err
	}

	var servicesList []string
	for _, service := range services {
		if len(service.Name) > 0 {
			servicesList = append(servicesList, service.Name)
		}
	}

	return servicesList, nil
}

func GetListenAddresses(consulServiceIp string) ([]string, error) {
	listenAddresses = []string{}
	servicesList, err := getNormalConsulServices(consulServiceIp)
	if err != nil {
		return nil, errors.New("failed to get consul services")
	}

	for _, service := range servicesList {
		addresses, err := getListenAddress(consulServiceIp, service)
		if err != nil {
			return nil, errors.New("failed to get worker addresses")
		}

		if !containsAny(listenAddresses, addresses) {
			listenAddresses = append(listenAddresses, addresses...)
		}
	}
	return listenAddresses, nil
}
