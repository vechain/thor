// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"os"
	"strings"
)

const GalacticaASCIIArt = `
                                                      /\ /\
                                                     /  \---._
                                                    / / ` + "`" + `     ` + "`" + `\
                                                    \ \     <@)@)      
                                                    /` + "`" + `         ~ ~._ 
                                                   /                ` + "`" + `() 
                                                  /     \            /
                                                 /       |` + "`" + `\_       /
                                             ___/________|_  ` + "`" + `—____/
                                            (______________)
                                            _/~          | 
                                          _/~             \  
                                        _/~               |
                                      _/~                 |
                                    _/~                   |
                                  _/~         ~.          |
                                _/~             \        /\
                             __/~               /` + "`" + `\     ` + "`" + `||
                           _/~      ~~-._     /~   \     ||
                          /~             ~./~'      \    |)
                         /                 ~.        \   )|
                        /                    :       |   ||
                        |                    :       |   ||
                        |                   .'       |   ||
                   __.-` + "`" + `                __.'--.      |   |` + "`" + `---. 
                .-~  ___.         __.--~` + "`" + `--.))))     |   ` + "`" + `---.)))
               ` + "`" + `---~~     ` + "`" + `-...--.________)))))      \_____)))))


   ______       _       _____          _        ______  _________  _____   ______       _       
 .' ___  |     / \     |_   _|        / \     .' ___  ||  _   _  ||_   _|.' ___  |     / \      
/ .'   \_|    / _ \      | |         / _ \   / .'   \_||_/ | | \_|  | | / .'   \_|    / _ \     
| |   ____   / ___ \     | |   _    / ___ \  | |           | |      | | | |          / ___ \    
\ ` + "`" + `.___]  |_/ /   \ \_  _| |__/ | _/ /   \ \_\ ` + "`" + `.___.'\   _| |_    _| |_\ ` + "`" + `.___.'\ _/ /   \ \_  
 ` + "`" + `._____.'|____| |____||________||____| |____|` + "`" + `.____ .'  |_____|  |_____|` + "`" + `.____ .'|____| |____| 
        _        ______  _________  _____  ____   ____  _     _________  ________  ______    
       / \     .' ___  ||  _   _  ||_   _||_  _| |_  _|/ \   |  _   _  ||_   __  ||_   _ ` + "`" + `
      / _ \   / .'   \_||_/ | | \_|  | |    \ \   / / / _ \  |_/ | | \_|  | |_ \_|  | | ` + "`" + `. \ 
     / ___ \  | |           | |      | |     \ \ / / / ___ \     | |      |  _| _   | |  | | 
   _/ /   \ \_\ ` + "`" + `.___.'\   _| |_    _| |_     \ ' /_/ /   \ \_  _| |_    _| |__/ | _| |_.' / 
  |____| |____|` + "`" + `.____ .'  |_____|  |_____|     \_/|____| |____||_____|  |________||______.'
`

var printed bool

func printGalacticaWelcomeInfo() {
	if !printed {
		// remove the leading new line symbol as previous printed log will have a new line symbol
		str, _ := strings.CutPrefix(GalacticaASCIIArt, "\n")
		os.Stdout.WriteString(str)
		printed = true
	}
}
