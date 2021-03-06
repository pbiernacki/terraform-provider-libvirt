package libvirt

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform/helper/acctest"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	libvirt "github.com/libvirt/libvirt-go"
)

func testAccCheckLibvirtVolumeExists(name string, volume *libvirt.StorageVol) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		virConn := testAccProvider.Meta().(*Client).libvirt

		rs, err := getResourceFromTerraformState(name, state)
		if err != nil {
			return err
		}

		retrievedVol, err := getVolumeFromTerraformState(name, state, *virConn)
		if err != nil {
			return err
		}

		realID, err := retrievedVol.GetKey()
		if err != nil {
			return err
		}

		if realID != rs.Primary.ID {
			return fmt.Errorf("Resource ID and volume key does not match")
		}

		*volume = *retrievedVol

		return nil
	}
}

func testAccCheckLibvirtVolumeDoesNotExists(n string, volume *libvirt.StorageVol) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		virConn := testAccProvider.Meta().(*Client).libvirt

		key, err := volume.GetKey()
		if err != nil {
			return fmt.Errorf("Can't retrieve volume key: %s", err)
		}

		vol, err := virConn.LookupStorageVolByKey(key)
		if err == nil {
			vol.Free()
			return fmt.Errorf("Volume '%s' still exists", key)
		}

		return nil
	}
}

func testAccCheckLibvirtVolumeIsBackingStore(name string, volume *libvirt.StorageVol) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		virConn := testAccProvider.Meta().(*Client).libvirt

		vol, err := getVolumeFromTerraformState(name, state, *virConn)
		if err != nil {
			return err
		}

		volXMLDesc, err := vol.GetXMLDesc(0)
		if err != nil {
			return fmt.Errorf("Error retrieving libvirt volume XML description: %s", err)
		}

		volumeDef := newDefVolume()
		err = xml.Unmarshal([]byte(volXMLDesc), &volumeDef)
		if err != nil {
			return fmt.Errorf("Error reading libvirt volume XML description: %s", err)
		}
		if volumeDef.BackingStore == nil {
			return fmt.Errorf("FAIL: the volume was supposed to be a backingstore, but it is not")
		}
		value := volumeDef.BackingStore.Path
		if value == "" {
			return fmt.Errorf("FAIL: the volume was supposed to be a backingstore, but it is not")
		}

		return nil
	}
}

func TestAccLibvirtVolume_Basic(t *testing.T) {
	var volume libvirt.StorageVol
	randomVolumeResource := acctest.RandString(10)
	randomVolumeName := acctest.RandString(10)
	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testaccCheckLibvirtDestroyResource("libvirt_volume", *testAccProvider.Meta().(*Client).libvirt),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
				resource "libvirt_volume" "%s" {
					name = "%s"
					size =  1073741824
				}`, randomVolumeResource, randomVolumeName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckLibvirtVolumeExists("libvirt_volume."+randomVolumeResource, &volume),
					resource.TestCheckResourceAttr(
						"libvirt_volume."+randomVolumeResource, "name", randomVolumeName),
					resource.TestCheckResourceAttr(
						"libvirt_volume."+randomVolumeResource, "size", "1073741824"),
				),
			},
		},
	})
}

func TestAccLibvirtVolume_BackingStoreTestByID(t *testing.T) {
	var volume libvirt.StorageVol
	var volume2 libvirt.StorageVol
	randomVolumeResource := acctest.RandString(10)
	randomVolumeName := acctest.RandString(10)
	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testaccCheckLibvirtDestroyResource("libvirt_volume", *testAccProvider.Meta().(*Client).libvirt),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
				resource "libvirt_volume" "%s" {
					name = "%s"
					size =  1073741824
				}
				resource "libvirt_volume" "backing-store" {
					name = "backing-store"
					base_volume_id = "${libvirt_volume.%s.id}"
			        }
				`, randomVolumeResource, randomVolumeName, randomVolumeResource),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckLibvirtVolumeExists("libvirt_volume."+randomVolumeResource, &volume),
					testAccCheckLibvirtVolumeIsBackingStore("libvirt_volume.backing-store", &volume2),
				),
			},
		},
	})
}

func TestAccLibvirtVolume_BackingStoreTestByName(t *testing.T) {
	var volume libvirt.StorageVol
	var volume2 libvirt.StorageVol
	randomVolumeResource := acctest.RandString(10)
	randomVolumeName := acctest.RandString(10)
	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testaccCheckLibvirtDestroyResource("libvirt_volume", *testAccProvider.Meta().(*Client).libvirt),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "libvirt_volume" "%s" {
						name = "%s"
						size =  1073741824
					}
					resource "libvirt_volume" "backing-store" {
						name = "backing-store"
						base_volume_name = "${libvirt_volume.%s.name}"
				  }	`, randomVolumeResource, randomVolumeName, randomVolumeResource),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckLibvirtVolumeExists("libvirt_volume."+randomVolumeResource, &volume),
					testAccCheckLibvirtVolumeIsBackingStore("libvirt_volume.backing-store", &volume2),
				),
			},
		},
	})
}

// The destroy function should always handle the case where the resource might already be destroyed
// (manually, for example). If the resource is already destroyed, this should not return an error.
// This allows Terraform users to manually delete resources without breaking Terraform.
// This test should fail without a proper "Exists" implementation
func TestAccLibvirtVolume_ManuallyDestroyed(t *testing.T) {
	var volume libvirt.StorageVol
	randomVolumeResource := acctest.RandString(10)
	randomVolumeName := acctest.RandString(10)
	testAccCheckLibvirtVolumeConfigBasic := fmt.Sprintf(`
	resource "libvirt_volume" "%s" {
		name = "%s"
		size =  1073741824
	}`, randomVolumeResource, randomVolumeName)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testaccCheckLibvirtDestroyResource("libvirt_volume", *testAccProvider.Meta().(*Client).libvirt),
		Steps: []resource.TestStep{
			{
				Config: testAccCheckLibvirtVolumeConfigBasic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckLibvirtVolumeExists("libvirt_volume."+randomVolumeResource, &volume),
				),
			},
			{
				Config:  testAccCheckLibvirtVolumeConfigBasic,
				Destroy: true,
				PreConfig: func() {
					client := testAccProvider.Meta().(*Client)
					id, err := volume.GetKey()
					if err != nil {
						panic(err)
					}
					removeVolume(client, id)
				},
			},
		},
	})
}

func TestAccLibvirtVolume_UniqueName(t *testing.T) {
	randomVolumeName := acctest.RandString(10)
	randomVolumeResource2 := acctest.RandString(10)
	randomVolumeResource := acctest.RandString(10)
	config := fmt.Sprintf(`
	resource "libvirt_volume" "%s" {
		name = "%s"
		size =  1073741824
	}

	resource "libvirt_volume" "%s" {
		name = "%s"
		size =  1073741824
	}
	`, randomVolumeResource, randomVolumeName, randomVolumeResource2, randomVolumeName)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testaccCheckLibvirtDestroyResource("libvirt_volume", *testAccProvider.Meta().(*Client).libvirt),
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`storage volume '` + randomVolumeName + `' already exists`),
			},
		},
	})
}

func TestAccLibvirtVolume_DownloadFromSource(t *testing.T) {
	var volume libvirt.StorageVol
	randomVolumeResource := acctest.RandString(10)
	randomVolumeName := acctest.RandString(10)

	fws := fileWebServer{}
	if err := fws.Start(); err != nil {
		t.Fatal(err)
	}
	defer fws.Stop()

	content := []byte("a fake image")
	url, _, err := fws.AddContent(content)
	if err != nil {
		t.Fatal(err)
	}

	config := fmt.Sprintf(`
	resource "libvirt_volume" "%s" {
		name   = "%s"
		source = "%s"
	}`, randomVolumeResource, randomVolumeName, url)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testaccCheckLibvirtDestroyResource("libvirt_volume", *testAccProvider.Meta().(*Client).libvirt),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckLibvirtVolumeExists("libvirt_volume."+randomVolumeResource, &volume),
					resource.TestCheckResourceAttr(
						"libvirt_volume."+randomVolumeResource, "name", randomVolumeName),
				),
			},
		},
	})
}

func TestAccLibvirtVolume_DownloadFromSourceFormat(t *testing.T) {
	var volumeRaw libvirt.StorageVol
	var volumeQCOW2 libvirt.StorageVol
	randomVolumeNameRaw := acctest.RandString(10)
	randomVolumeNameQCOW := acctest.RandString(10)
	randomVolumeResourceRaw := acctest.RandString(10)
	randomVolumeResourceQCOW := acctest.RandString(10)
	qcow2Path, err := filepath.Abs("testdata/test.qcow2")
	if err != nil {
		t.Fatal(err)
	}

	rawPath, err := filepath.Abs("testdata/initrd.img")
	if err != nil {
		t.Fatal(err)
	}

	config := fmt.Sprintf(`
	resource "libvirt_volume" "%s" {
		name   = "%s"
		source = "%s"
	}
  resource "libvirt_volume" "%s" {
		name   = "%s"
		source = "%s"
	}`, randomVolumeResourceRaw, randomVolumeNameRaw, fmt.Sprintf("file://%s", rawPath), randomVolumeResourceQCOW, randomVolumeNameQCOW, fmt.Sprintf("file://%s", qcow2Path))
	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testaccCheckLibvirtDestroyResource("libvirt_volume", *testAccProvider.Meta().(*Client).libvirt),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckLibvirtVolumeExists("libvirt_volume."+randomVolumeResourceRaw, &volumeRaw),
					testAccCheckLibvirtVolumeExists("libvirt_volume."+randomVolumeResourceQCOW, &volumeQCOW2),
					resource.TestCheckResourceAttr(
						"libvirt_volume."+randomVolumeResourceRaw, "name", randomVolumeNameRaw),
					resource.TestCheckResourceAttr(
						"libvirt_volume."+randomVolumeResourceRaw, "format", "raw"),
					resource.TestCheckResourceAttr(
						"libvirt_volume."+randomVolumeResourceQCOW, "name", randomVolumeNameQCOW),
					resource.TestCheckResourceAttr(
						"libvirt_volume."+randomVolumeResourceQCOW, "format", "qcow2"),
				),
			},
		},
	})
}

func TestAccLibvirtVolume_Format(t *testing.T) {
	var volume libvirt.StorageVol
	randomVolumeResource := acctest.RandString(10)
	randomVolumeName := acctest.RandString(10)

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testaccCheckLibvirtDestroyResource("libvirt_volume", *testAccProvider.Meta().(*Client).libvirt),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
				resource "libvirt_volume" "%s" {
					name   = "%s"
					format = "raw"
					size   =  1073741824
				}`, randomVolumeResource, randomVolumeName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckLibvirtVolumeExists("libvirt_volume."+randomVolumeResource, &volume),
					resource.TestCheckResourceAttr(
						"libvirt_volume."+randomVolumeResource, "name", randomVolumeName),
					resource.TestCheckResourceAttr(
						"libvirt_volume."+randomVolumeResource, "size", "1073741824"),
					resource.TestCheckResourceAttr(
						"libvirt_volume."+randomVolumeResource, "format", "raw"),
				),
			},
		},
	})
}

func TestAccLibvirtVolume_Import(t *testing.T) {
	var volume libvirt.StorageVol
	randomVolumeResource := acctest.RandString(10)
	randomVolumeName := acctest.RandString(10)
	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testaccCheckLibvirtDestroyResource("libvirt_volume", *testAccProvider.Meta().(*Client).libvirt),
		Steps: []resource.TestStep{
			resource.TestStep{
				Config: fmt.Sprintf(`
					resource "libvirt_volume" "%s" {
							name   = "%s"
							format = "raw"
							size   =  1073741824
					}`, randomVolumeResource, randomVolumeName),
			},
			resource.TestStep{
				ResourceName: "libvirt_volume." + randomVolumeResource,
				ImportState:  true,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckLibvirtVolumeExists("libvirt_volume."+randomVolumeResource, &volume),
					resource.TestCheckResourceAttr(
						"libvirt_volume."+randomVolumeResource, "name", randomVolumeName),
					resource.TestCheckResourceAttr(
						"libvirt_volume."+randomVolumeResource, "size", "1073741824"),
				),
			},
		},
	})
}
