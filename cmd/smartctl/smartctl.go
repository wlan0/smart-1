// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Go SMART library smartctl reference implementation.
//
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/dswarbrick/smart"
	"github.com/dswarbrick/smart/cmd/smartctl/smartdb"
	"github.com/dswarbrick/smart/drivedb"
	"github.com/dswarbrick/smart/megaraid"
	"github.com/dswarbrick/smart/nvme"
	"github.com/dswarbrick/smart/scsi"
)

const (
	_LINUX_CAPABILITY_VERSION_3 = 0x20080522

	CAP_SYS_RAWIO = 1 << 17
	CAP_SYS_ADMIN = 1 << 21
)

type capHeader struct {
	version uint32
	pid     int
}

type capData struct {
	effective   uint32
	permitted   uint32
	inheritable uint32
}

type capsV3 struct {
	hdr  capHeader
	data [2]capData
}

// checkCaps invokes the capget syscall to check for necessary capabilities. Note that this depends
// on the binary having the capabilities set (i.e., via the `setcap` utility), and on VFS support.
// Alternatively, if the binary is executed as root, it automatically has all capabilities set.
func checkCaps() {
	caps := new(capsV3)
	caps.hdr.version = _LINUX_CAPABILITY_VERSION_3

	// Use RawSyscall since we do not expect it to block
	_, _, e1 := unix.RawSyscall(unix.SYS_CAPGET, uintptr(unsafe.Pointer(&caps.hdr)), uintptr(unsafe.Pointer(&caps.data)), 0)
	if e1 != 0 {
		fmt.Println("capget() failed:", e1.Error())
		return
	}

	if (caps.data[0].effective&CAP_SYS_RAWIO == 0) && (caps.data[0].effective&CAP_SYS_ADMIN == 0) {
		fmt.Println("Neither cap_sys_rawio nor cap_sys_admin are in effect. Device access will probably fail.")
	}
}

func scanDevices() {
	for _, device := range smart.ScanDevices() {
		fmt.Printf("%#v\n", device)
	}

	// Open megaraid_sas ioctl device and scan for hosts / devices
	if m, err := megaraid.CreateMegasasIoctl(); err == nil {
		defer m.Close()
		for _, device := range m.ScanDevices() {
			fmt.Printf("%#v\n", device)
		}
	}

	//smart.MegaScan()
}

func main() {
	fmt.Println("Go smartctl Reference Implementation")
	fmt.Printf("Built with %s on %s (%s)\n\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	device := flag.String("device", "", "SATA / NVMe device from which to read SMART attributes, e.g., /dev/sda, /dev/nvme0")
	megaraidDev := flag.String("megaraid", "", "MegaRAID host and device ID from which to read SMART attributes, e.g., megaraid0_23")
	scan := flag.Bool("scan", false, "Scan for drives that support SMART")
	flag.Parse()

	checkCaps()

	if *device != "" {
		var (
			d   scsi.Device // interface
			err error
		)

		if strings.HasPrefix(*device, "/dev/nvme") {
			d = nvme.NewNVMeDevice(*device)
			err = d.Open()
		} else {
			d, err = scsi.OpenSCSIAutodetect(*device)
		}

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		defer d.Close()

		db, err := drivedb.OpenDriveDbFromReader(bytes.NewBuffer(smartdb.MustAsset("drivedb.yaml")))
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := d.PrintSMART(&db, os.Stdout); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	} else if *megaraidDev != "" {
		var (
			host uint16
			disk uint8
		)

		if _, err := fmt.Sscanf(*megaraidDev, "megaraid%d_%d", &host, &disk); err != nil {
			fmt.Println("Invalid MegaRAID host / device ID syntax")
			os.Exit(1)
		}

		megaraid.OpenMegasasIoctl(host, disk)
	} else if *scan {
		scanDevices()
	} else {
		flag.PrintDefaults()
		os.Exit(1)
	}
}
