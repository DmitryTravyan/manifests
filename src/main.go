package main

import (
	"fmt"
	"io/ioutil"
	"os/exec"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type yandexCluster struct {
    Var string
}

func main() {
    var err error
    var bytes []byte
    
    llog.SetLevel(llog.TraceLevel)

    cluster := yandexCluster{""}

    if bytes, err = cluster.deployStorage(); err != nil {
        panic(err)
    }

    llog.Infof("%s", string(bytes))

    if bytes, err = cluster.deployDatabase(); err != nil {
        panic(err)
    }

    llog.Infof("%s", string(bytes))
}

// parse manifest and deploy yandex db storage via k8s go-client
func (yc *yandexCluster) deployStorage() ([]byte, error) {
	var err error
	var bytes []byte
	var storage map[string]interface{}
	var configuration map[string]interface{}

	bytes, err = ioutil.ReadFile("storage.yml")
	if err != nil {
		return []byte{}, merry.Prepend(err, "Error then reading file")
	}
	llog.Tracef("%v bytes readed from storage.yml\n", len(bytes))

	if err = yaml.Unmarshal(bytes, &storage); err != nil {
		return []byte{}, merry.Prepend(err, "Error then unmarshaling storage manifest")
	}
	spec := storage["spec"].(map[string]interface{})
	spec["resources"].(map[string]interface{})["limits"] = map[string]interface{}{
		"cpu":    "1000m",  //  TODO replase to formula based on host resources
		"memory": "4096Mi", // resources can fe fetched from terraform.tfstate
	}
	spec["nodes"] = 4 //  TODO get it from terraform.tfstate

	if err = yaml.Unmarshal(
		[]byte(storage["spec"].(map[string]interface{})["configuration"].(string)),
		&configuration,
	); err != nil {
		return []byte{}, merry.Prepend(err, "Error then unmarshaling storage manifest")
	}

	// TODO replace to config based on resorces from terraform.tfstate
	drive := configuration["host_configs"].([]interface{})[0].(map[string]interface{})["drive"]
	drive.([]interface{})[0] = map[string]interface{}{
		"path": "SectorMap:1:64",
		"type": "SSD",
	}

	// TODO replace to config based on resorces from terraform.tfstate
	ring := configuration["domains_config"].(map[string]interface{})["state_storage"]
	ring.([]interface{})[0] = map[string]interface{}{
		"ring": map[string]interface{}{
			"node": []interface{}{
				1,
			},
			"nto_select": 1,
		},
		"ssid": 1,
	}

	// TODO replace to config based on resorces from terraform.tfstate
	serviceSet := configuration["blob_storage_config"].(map[string]interface{})["service_set"]
	serviceSet.(map[string]interface{})["groups"] = []interface{}{
		map[string]interface{}{
			"erasure_species": "none",
			"rings": []interface{}{
				map[string]interface{}{
					"fail_domains": []interface{}{
						map[string]interface{}{
							"vdisk_locations": []interface{}{
								map[string]interface{}{
									"node_id":        1,
									"path":           "SectorMap:1:64",
									"pdisk_category": "SSD",
								},
							},
						},
					},
				},
			},
		},
	}
	// TODO replace to config based on resorces from terraform.tfstate
	profile := configuration["channel_profile_config"].(map[string]interface{})["profile"]
	profile.([]interface{})[0] = map[string]interface{}{
		"channel_profile_config": []interface{}{
			map[string]interface{}{
				"channel": []interface{}{
					map[string]interface{}{
						"erasure_species":   "none",
						"pdisk_category":    1,
						"storage_pool_kind": "ssd",
					},
					map[string]interface{}{
						"erasure_species":   "none",
						"pdisk_category":    1,
						"storage_pool_kind": "ssd",
					},
					map[string]interface{}{
						"erasure_species":   "none",
						"pdisk_category":    1,
						"storage_pool_kind": "ssd",
					},
				},
			},
		},
	}

	if bytes, err = yaml.Marshal(configuration); err != nil {
		return []byte{}, merry.Prepend(err, "Error then serializing storage configuration")
	}
	storage["spec"].(map[string]interface{})["configuration"] = string(bytes)

	if bytes, err = yaml.Marshal(storage); err != nil {
		return []byte{}, merry.Prepend(err, "Error then serializing storage")
	}

	if err = ioutil.WriteFile("new-storage.yml", bytes, 0644); err != nil {
		return []byte{}, merry.Prepend(err, "Error then writing storage.yml")
	}

	//return applyManifest("storage.yml")
    return bytes, err
}

func (yc *yandexCluster) deployDatabase() ([]byte, error) {
	var err error
	var bytes []byte
	var storage map[string]interface{}

	bytes, err = ioutil.ReadFile("database.yml")
	if err != nil {
		return []byte{}, merry.Prepend(err, "Error then reading database.yml")
	}
	llog.Tracef("%v bytes readed from database.yml\n", len(bytes))

	err = yaml.Unmarshal(bytes, &storage)
	if err != nil {
		return []byte{}, merry.Prepend(err, "Error then unmarshaling database manifest")
	}

	//  TODO get it from terraform.tfstate
	storage["spec"].(map[string]interface{})["nodes"] = 1

	resources := storage["spec"].(map[string]interface{})["resources"].(map[string]interface{})
	resources["containerResources"].(map[string]interface{})["limits"] = map[string]interface{}{
		"cpu":    "500m",   //  TODO replase to formula based on host resources
		"memory": "1024Mi", //  resources can fe fetched from terraform.tfstate
	}
	resources["storageUnits"] = []interface{}{
		map[string]interface{}{
			"count":    1,     //  TODO replase to formula based on host resources
			"unitKind": "ssd", //  resources can fe fetched from terraform.tfstate
		},
	}
	if bytes, err = yaml.Marshal(storage); err != nil {
		return []byte{}, merry.Prepend(err, "Error then serializing database.yml")
	}

	if err = ioutil.WriteFile("new-database.yml", bytes, 0644); err != nil {
		return []byte{}, merry.Prepend(err, "Error then writing database.yml")
	}

	//return applyManifest("database.yml")

    return bytes, err
}

/// Run kubectl and apply manifest
func applyManifest(manifestName string) ([]byte, error) {
	var err error
	var cmd *exec.Cmd
	var stdout []byte
	cmd = exec.Command("kubectl", "apply", "-f", manifestName)
	if stdout, err = cmd.Output(); err != nil {
		return stdout, merry.Prepend(
			err,
			fmt.Sprintf("Error then applying %s manifest", manifestName),
		)
	}
	llog.Debug(string(stdout))
	return stdout, err
}
