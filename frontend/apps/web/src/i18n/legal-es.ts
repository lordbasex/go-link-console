// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// Spanish translation of legal-en.ts (same shape: LegalTexts).
import type { LegalTexts } from "./legal-en";

export const legalEs: LegalTexts = {
  updated: "Versión del 11 de octubre de 2026",
  prevails: "Esta página es una traducción. Si difiere de la versión en inglés, prevalece la versión en inglés.",
  contact: "Consultas, pedidos y reportes: lord.basex@gmail.com",
  terms: {
    title: "Términos de uso",
    intro:
      "go-link Console es software gratuito, de código abierto, independiente y sin fines de lucro. Un anfitrión ejecuta la app de go-link Console en su propia computadora, con sus propios archivos de juegos, e invita a amigos que juegan desde el navegador. El sitio web y el servidor de señalización del proyecto solo presentan al anfitrión y a los invitados entre sí. Al vincular un dispositivo, unirte a una partida o usar go-link Console de cualquier otra forma, aceptas estos términos. Si no los aceptas, no uses go-link Console.",
    sections: [
      {
        id: "roms",
        title: "ROMs y contenido con derechos de autor: política de contenido cero",
        body: [],
        list: [
          "go-link Console no aloja, incluye, distribuye, vende, enlaza ni descarga ROMs, imágenes de juegos, arte, carátulas ni archivos de BIOS, y no nombra ni señala ningún sitio que los ofrezca.",
          "go-link Console es una herramienta de software y un motor de transmisión. Cada archivo de juego queda en la computadora del anfitrión; ningún servidor de go-link Console lo recibe, lo guarda ni lo retransmite.",
          "El anfitrión es el único responsable de los archivos que carga. Solo puede usar archivos que tenga derecho legal a usar, como copias de seguridad de placas arcade o software de su propiedad. La reproducción o transmisión no autorizada de material protegido es responsabilidad exclusiva del anfitrión que la realiza.",
          "Transmitir un juego a invitados puede considerarse una comunicación pública de ese juego según las leyes de tu país. El anfitrión es responsable de asegurarse de que puede hacerlo.",
        ],
      },
      {
        id: "use",
        title: "Uso aceptable",
        body: ["No puedes usar go-link Console, su sitio, su servidor de señalización ni su relay para:"],
        list: [
          "transmitir, compartir o enviar contenido ilegal, que infrinja derechos, pornográfico, difamatorio, acosador, de odio, amenazante o malicioso, por video, voz o chat;",
          "acosar, suplantar o dañar a otras personas, o recopilar sus datos sin su consentimiento;",
          "atacar, sobrecargar, sondear o abusar de los servidores del proyecto, por ejemplo usando el relay como proxy de uso general o eludiendo sus límites;",
          "obtener dinero del núcleo del emulador o de los juegos.",
        ],
      },
      {
        id: "rooms",
        title: "Los anfitriones y sus salas",
        body: [
          "El proyecto puede bloquear conexiones que no cumplan estas reglas. Los anfitriones son responsables de a quién invitan y de lo que pasa en sus salas: pueden sacar personas, apagar el chat y cerrar salas en cualquier momento.",
        ],
        list: [],
      },
      {
        id: "age",
        title: "Edad",
        body: [
          "Debes tener al menos 13 años, o la edad mínima que exija tu país para usar servicios en línea sin el consentimiento de tus padres. Las personas menores solo pueden usar go-link Console con un padre, madre o tutor que acepte estos términos.",
        ],
        list: [],
      },
      {
        id: "warranty",
        title: "Sin garantía",
        body: [
          "go-link Console, el sitio, el servidor de señalización y el relay se ofrecen \"tal cual\" y \"según disponibilidad\", sin garantías de ningún tipo, expresas o implícitas, incluidas las de comerciabilidad, idoneidad para un fin determinado, disponibilidad y no infracción. El servicio puede cambiar, fallar, perder conexiones, tener latencia o detenerse en cualquier momento, sin aviso.",
        ],
        list: [],
      },
      {
        id: "liability",
        title: "Limitación de responsabilidad",
        body: [
          "En la máxima medida que permita la ley, los autores y colaboradores de go-link Console no son responsables de ningún daño directo, indirecto, incidental, especial, consecuente o punitivo, ni de pérdidas de datos, juegos, partidas guardadas o ganancias, derivados del uso o de la imposibilidad de usar go-link Console o sus servicios, ni del contenido que anfitriones o invitados carguen, transmitan o envíen.",
        ],
        list: [],
      },
      {
        id: "indemnity",
        title: "Indemnidad",
        body: [
          "Si tu uso de go-link Console incumple estos términos o la ley, por ejemplo al transmitir archivos que no tenías derecho a usar, aceptas mantener indemnes a los autores y colaboradores de go-link Console frente a cualquier reclamo que surja de ello.",
        ],
        list: [],
      },
      {
        id: "licenses",
        title: "Licencias",
        body: [],
        list: [
          "El código fuente propio de go-link Console se publica bajo la licencia MIT.",
          "Los componentes de terceros incluidos en la app y el sitio de go-link Console conservan sus propias licencias, listadas en THIRD_PARTY_NOTICES.md e incluidas en cada versión.",
          "El núcleo del emulador no es parte de go-link Console: la app descarga mame2003-plus del buildbot de libretro cuando el anfitrión lo pide. mame2003-plus está bajo la licencia no comercial de MAME, así que usar go-link Console con ese núcleo es solo para uso no comercial: no se permite ninguna distribución comercial ni monetización de go-link Console junto con ese núcleo.",
        ],
      },
      {
        id: "trademarks",
        title: "Marcas",
        body: [],
        list: [
          "MAME® es una marca registrada de Gregory Ember. go-link Console no está afiliado, respaldado ni patrocinado por MAMEdev, el equipo de MAME ni libretro. El nombre MAME solo se usa para describir con qué es compatible go-link Console, y no se usa el logo de MAME.",
          "Los nombres de empresas, títulos de juegos, logos y personajes que los juegos puedan mostrar son marcas y propiedad de sus respectivos dueños. go-link Console es independiente y no declara ninguna afiliación ni respaldo de ninguno de ellos.",
        ],
      },
      {
        id: "reports",
        title: "Reportes y cambios",
        body: [
          "go-link Console no aloja juegos ni contenido de usuarios, así que no puede quitarlos. Para reportar abusos de los servidores del proyecto, o cualquier otra cosa sobre estos términos, escribe a lord.basex@gmail.com.",
          "Estos términos pueden cambiar. La fecha de la versión está arriba de esta página, y el sitio te pide aceptar una versión nueva antes de volver a vincular un dispositivo o unirte a una partida.",
        ],
        list: [],
      },
    ],
  },
  privacy: {
    title: "Política de privacidad",
    intro:
      "go-link Console está pensado para recopilar lo mínimo posible. No tiene cuentas, cookies, analítica, publicidad ni rastreadores de terceros, y las tipografías del sitio las sirve el propio sitio.",
    sections: [
      {
        id: "p2p",
        title: "Juegos, voz y chat viajan punto a punto",
        body: [],
        list: [
          "El video, el sonido del juego, la voz, los controles y el chat viajan directamente entre la app del anfitrión y el navegador de cada invitado por WebRTC, que siempre va cifrado (DTLS y SRTP).",
          "Cuando no es posible una conexión directa, ese tráfico pasa por el relay (TURN) del proyecto. Sigue cifrado de punta a punta: el relay reenvía paquetes que no puede leer.",
          "Los servidores del proyecto nunca graban, guardan ni procesan el video, el sonido, la voz ni el chat de una partida.",
        ],
      },
      {
        id: "signaling",
        title: "Qué ve el servidor de señalización",
        body: ["Para presentar al anfitrión y a los invitados, el servidor de señalización procesa, solo en memoria y mientras dura la conexión:"],
        list: [
          "la dirección IP de cada conexión, para conectarla y aplicar límites contra abusos;",
          "identificadores aleatorios de conexión, códigos de vinculación, identificadores de sala e invitaciones;",
          "la negociación de WebRTC (que incluye direcciones de red) y el PIN de sala que escribe un invitado, que reenvía sin guardarlo;",
          "el identificador de la app y un hash de su secreto, guardados en memoria hasta 30 días para proteger la identidad de la app.",
        ],
      },
      {
        id: "logs",
        title: "Registros técnicos",
        body: [
          "Nada de lo anterior se escribe en disco ni se conserva después de reiniciar el servidor de señalización, y este no registra direcciones IP. El relay guarda registros técnicos que pueden incluir direcciones IP (por ejemplo, intentos de conexión bloqueados); rotan automáticamente, tienen un tamaño máximo y solo se usan para operar y proteger el servicio. El sitio lo sirve un proveedor de hosting que procesa los datos habituales de cada pedido (dirección IP, navegador, fecha) para entregar las páginas.",
        ],
        list: [],
      },
      {
        id: "browser",
        title: "Qué guarda tu navegador",
        body: [
          "El sitio guarda preferencias y claves en el almacenamiento local de tu navegador, nunca en cookies ni en nuestros servidores: idioma, tema, el servidor de señalización que elegiste, el vínculo con tu app, los pases para volver a partidas a las que entraste, la clave del panel local, ajustes de vista y sonido, y la fecha en que aceptaste estos términos. El editor de skins y Willy Maker guardan tus skins y tus juegos de la misma forma, en el almacenamiento local y en la base de datos propia del navegador (IndexedDB), y nunca salen de tu navegador salvo que los descargues. Al borrar los datos de este sitio en tu navegador, se elimina todo.",
        ],
        list: [],
      },
      {
        id: "host",
        title: "Qué guarda la app del anfitrión",
        body: ["La app del anfitrión corre en su propia computadora, y el anfitrión controla sus datos. Guarda, solo en esa computadora:"],
        list: [
          "los navegadores vinculados (solo un hash de cada clave) y la versión de estos términos que aceptó cada uno;",
          "el historial de partidas: cada sala que se jugó y, por cada invitado, el nombre que escribió, los puestos en los que jugó, el camino de conexión y la dirección IP que vio la app (vacía cuando el invitado entró por el relay);",
          "el reporte de red de cada sala: desde que la sala se prendió por primera vez hasta que se borra para siempre, una muestra por segundo de cómo se comportaron el juego y la conexión de cada invitado (ida y vuelta, paquetes perdidos, imagen congelada, el ritmo de los controles, paquetes de voz, la prueba de latencia) y un registro de eventos (quién entró y salió, el nombre que usó, el camino de conexión y la dirección IP, congelamientos); solo lo ve el anfitrión, y borrar la sala para siempre o un restablecimiento de fábrica lo borra;",
          "los últimos 50 mensajes del chat de cada sala mientras está abierta, en memoria;",
          "grabaciones, solo cuando el anfitrión graba una partida: la imagen y el sonido del juego y la voz de cada jugador, en un archivo en la computadora del anfitrión. Todos en la sala lo saben mientras se graba (un aviso REC y un mensaje en el chat). Una grabación se detiene sola a las 2 horas o 2 GB y cuando se pausa la partida. Solo el anfitrión puede descargarla o borrarla; borrar una partida del historial, vaciar el historial o reiniciar de fábrica borra sus grabaciones.",
        ],
      },
      {
        id: "addresses",
        title: "Direcciones de red entre jugadores",
        body: [
          "En una conexión punto a punto, la app del anfitrión y los invitados ven sus direcciones de red entre sí; así funciona WebRTC. Si no quieres compartir tu dirección con un anfitrión, no te unas a sus partidas.",
        ],
        list: [],
      },
      {
        id: "rights",
        title: "Tus derechos y los servidores propios",
        body: [
          "Como go-link Console no tiene cuentas ni guarda datos personales en sus servidores más allá de lo descrito aquí, la mayoría de los pedidos se resuelven borrando los datos de tu navegador o pidiéndole al anfitrión que borre su historial. Para cualquier consulta o pedido sobre tus datos, escribe a lord.basex@gmail.com.",
          "Cualquier persona puede usar su propio servidor de señalización. Si usas uno (el sitio lo muestra), su operador, y no el proyecto go-link Console, es responsable de él.",
        ],
        list: [],
      },
    ],
  },
  accept: {
    before: "Leí y acepto los ",
    terms: "Términos de uso",
    and: " y la ",
    privacy: "Política de privacidad",
    after: ", y solo usaré ROMs que tengo derecho legal a usar.",
    guestAfter: ".",
    required: "Acepta los términos de uso para continuar.",
  },
  footer: {
    tagline: "Juegos arcade transmitidos desde tu computadora a los navegadores de tus amigos. Gratis, de código abierto y sin fines de lucro.",
    product: "Producto",
    project: "Proyecto",
    legal: "Legal",
    source: "Código fuente",
    releases: "Descargas",
    license: "Licencia MIT",
    notices: "Licencias de terceros",
    legalDoc: "Texto legal completo",
    rights: "go-link Console. Código publicado bajo la licencia MIT.",
    trademark:
      "MAME® es una marca registrada de Gregory Ember. go-link Console no está afiliado ni respaldado por MAMEdev ni libretro. Los nombres y logos de los juegos pertenecen a sus dueños.",
    version: "Versión del sitio",
    github: "go-link Console en GitHub",
    noRoms: "go-link Console no aloja, incluye ni descarga ninguna ROM, BIOS ni imagen de juego.",
  },
};
