package getcid

import (
	"fmt"
	"os"

	"github.com/starlinglab/integrity-v2/util"
)

func Run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("specify the path to one file")
	}
	f, err := os.Open(args[0])
	if err != nil {
		return err
	}
	defer f.Close()
	cid, err := util.CalculateFileCid(f)
	if err != nil {
		return err
	}
	fmt.Println(cid)
	return nil
}
