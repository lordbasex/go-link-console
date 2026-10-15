// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !headless

package gui

// The texts of the update window, its tray item and Settings' Updates.
func init() {
	add := func(lang string, m map[string]string) {
		for k, v := range m {
			catalogs[lang][k] = v
		}
	}
	add("es", map[string]string{
		"Check for Updates…":              "Buscar actualizaciones…",
		"Update to %s…":                   "Actualizar a la %s…",
		"Update to go-link Console %s…":   "Actualizar a go-link Console %s…",
		"Updates":                         "Actualizaciones",
		"This is go-link Console %s.":     "Esta es la versión %s de go-link Console.",
		"Check for updates automatically": "Buscar actualizaciones automáticamente",
		"Every 6 hours go-link asks GitHub whether a newer version is out, and offers it. An update is installed only when its download carries go-link Console's signature.": "Cada 6 horas go-link le pregunta a GitHub si hay una versión más nueva y te la ofrece. Una actualización solo se instala si la descarga lleva la firma de go-link Console.",
		"Software update":             "Actualización",
		"Checking for updates…":       "Buscando actualizaciones…",
		"Could not check for updates": "No se pudo buscar actualizaciones",
		"There is no connection to GitHub, where the versions are. Try again later.": "No hay conexión con GitHub, donde están las versiones. Inténtalo más tarde.",
		"go-link Console is up to date":                                              "go-link Console está al día",
		"You have %s, the newest version.":                                           "Tienes la %s, la versión más nueva.",
		"OK":                                                                         "Aceptar",
		"You have %s.":                                                               "Tienes la %s.",
		"This copy cannot replace itself: download the new one from its page and install it as before.": "Esta copia no puede reemplazarse sola: descarga la nueva desde su página e instálala como la primera vez.",
		"Updating closes the rooms for a moment: each game is saved and comes back where it was.":       "Al actualizar, las salas se cierran un momento: cada partida se guarda y vuelve donde estaba.",
		"go-link Console %s is available":        "Ya está go-link Console %s",
		"What's new":                             "Novedades",
		"Later":                                  "Más tarde",
		"Download":                               "Descargar",
		"Update":                                 "Actualizar",
		"Downloading go-link Console %s…":        "Descargando go-link Console %s…",
		"Cancel":                                 "Cancelar",
		"Installing…":                            "Instalando…",
		"go-link Console opens again by itself.": "go-link Console se vuelve a abrir solo.",
		"The update was not installed":           "La actualización no se instaló",
		"The download is not the one go-link Console published (its signature or its contents differ), so it was thrown away.": "La descarga no es la que publicó go-link Console (su firma o su contenido no coinciden), así que se descartó.",
		"Something went wrong while downloading or installing it. Try again, or download it from its page.":                    "Algo falló al descargarla o instalarla. Vuelve a intentarlo, o descárgala desde su página.",
		"Open its page": "Abrir su página",
	})
	add("pt", map[string]string{
		"Check for Updates…":              "Procurar atualizações…",
		"Update to %s…":                   "Atualizar para a %s…",
		"Update to go-link Console %s…":   "Atualizar para o go-link Console %s…",
		"Updates":                         "Atualizações",
		"This is go-link Console %s.":     "Esta é a versão %s do go-link Console.",
		"Check for updates automatically": "Procurar atualizações automaticamente",
		"Every 6 hours go-link asks GitHub whether a newer version is out, and offers it. An update is installed only when its download carries go-link Console's signature.": "A cada 6 horas o go-link pergunta ao GitHub se há uma versão mais nova e a oferece. Uma atualização só é instalada quando o download tem a assinatura do go-link Console.",
		"Software update":             "Atualização",
		"Checking for updates…":       "Procurando atualizações…",
		"Could not check for updates": "Não foi possível procurar atualizações",
		"There is no connection to GitHub, where the versions are. Try again later.": "Não há conexão com o GitHub, onde estão as versões. Tente mais tarde.",
		"go-link Console is up to date":                                              "O go-link Console está atualizado",
		"You have %s, the newest version.":                                           "Você tem a %s, a versão mais nova.",
		"OK":                                                                         "OK",
		"You have %s.":                                                               "Você tem a %s.",
		"This copy cannot replace itself: download the new one from its page and install it as before.": "Esta cópia não pode se substituir sozinha: baixe a nova na página dela e instale como da primeira vez.",
		"Updating closes the rooms for a moment: each game is saved and comes back where it was.":       "Ao atualizar, as salas fecham por um momento: cada partida é salva e volta de onde estava.",
		"go-link Console %s is available":        "O go-link Console %s está disponível",
		"What's new":                             "Novidades",
		"Later":                                  "Mais tarde",
		"Download":                               "Baixar",
		"Update":                                 "Atualizar",
		"Downloading go-link Console %s…":        "Baixando o go-link Console %s…",
		"Cancel":                                 "Cancelar",
		"Installing…":                            "Instalando…",
		"go-link Console opens again by itself.": "O go-link Console abre de novo sozinho.",
		"The update was not installed":           "A atualização não foi instalada",
		"The download is not the one go-link Console published (its signature or its contents differ), so it was thrown away.": "O download não é o que o go-link Console publicou (a assinatura ou o conteúdo não batem), então foi descartado.",
		"Something went wrong while downloading or installing it. Try again, or download it from its page.":                    "Algo deu errado ao baixar ou instalar. Tente de novo, ou baixe na página dela.",
		"Open its page": "Abrir a página",
	})
}
