// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// Brazilian Portuguese translation of legal-en.ts (same shape: LegalTexts).
import type { LegalTexts } from "./legal-en";

export const legalPt: LegalTexts = {
  updated: "Versão de 11 de outubro de 2026",
  prevails: "Esta página é uma tradução. Se ela for diferente da versão em inglês, prevalece a versão em inglês.",
  contact: "Dúvidas, pedidos e denúncias: lord.basex@gmail.com",
  terms: {
    title: "Termos de uso",
    intro:
      "go-link Console é um software gratuito, de código aberto, independente e sem fins lucrativos. Um anfitrião roda o app go-link Console no próprio computador, com os próprios arquivos de jogos, e convida amigos que jogam pelo navegador. O site e o servidor de sinalização do projeto apenas apresentam o anfitrião e os convidados uns aos outros. Ao vincular um dispositivo, entrar em uma partida ou usar o go-link Console de qualquer outra forma, você aceita estes termos. Se não os aceitar, não use o go-link Console.",
    sections: [
      {
        id: "roms",
        title: "ROMs e conteúdo protegido: política de conteúdo zero",
        body: [],
        list: [
          "go-link Console não hospeda, inclui, distribui, vende, cria links nem baixa ROMs, imagens de jogos, arte, capas ou arquivos de BIOS, e não cita nem indica nenhum site que os ofereça.",
          "go-link Console é uma ferramenta de software e um motor de transmissão. Cada arquivo de jogo fica no computador do anfitrião; nenhum servidor do go-link Console o recebe, armazena ou retransmite.",
          "O anfitrião é o único responsável pelos arquivos que carrega. Ele só pode usar arquivos que tenha o direito legal de usar, como cópias de segurança de placas arcade ou software de sua propriedade. A reprodução ou transmissão não autorizada de material protegido é responsabilidade exclusiva do anfitrião que a realiza.",
          "Transmitir um jogo para convidados pode ser considerado comunicação pública desse jogo pelas leis do seu país. O anfitrião é responsável por garantir que pode fazê-lo.",
        ],
      },
      {
        id: "use",
        title: "Uso aceitável",
        body: ["Você não pode usar o go-link Console, seu site, seu servidor de sinalização ou seu relay para:"],
        list: [
          "transmitir, compartilhar ou enviar conteúdo ilegal, que viole direitos, pornográfico, difamatório, de assédio, de ódio, ameaçador ou malicioso, por vídeo, voz ou chat;",
          "assediar, se passar por outras pessoas ou prejudicá-las, ou coletar os dados delas sem consentimento;",
          "atacar, sobrecarregar, sondar ou abusar dos servidores do projeto, por exemplo usando o relay como proxy de uso geral ou burlando seus limites;",
          "ganhar dinheiro com o núcleo do emulador ou com os jogos.",
        ],
      },
      {
        id: "rooms",
        title: "Os anfitriões e suas salas",
        body: [
          "O projeto pode bloquear conexões que descumpram estas regras. Os anfitriões são responsáveis por quem convidam e pelo que acontece em suas salas: podem remover pessoas, desligar o chat e fechar salas a qualquer momento.",
        ],
        list: [],
      },
      {
        id: "age",
        title: "Idade",
        body: [
          "Você precisa ter pelo menos 13 anos, ou a idade mínima exigida no seu país para usar serviços online sem o consentimento dos pais. Pessoas mais novas só podem usar o go-link Console com um pai, mãe ou responsável que aceite estes termos.",
        ],
        list: [],
      },
      {
        id: "warranty",
        title: "Sem garantia",
        body: [
          "O go-link Console, o site, o servidor de sinalização e o relay são oferecidos \"no estado em que se encontram\" e \"conforme disponibilidade\", sem garantias de qualquer tipo, expressas ou implícitas, incluindo comercialização, adequação a uma finalidade específica, disponibilidade e não violação. O serviço pode mudar, falhar, perder conexões, ter latência ou parar a qualquer momento, sem aviso.",
        ],
        list: [],
      },
      {
        id: "liability",
        title: "Limitação de responsabilidade",
        body: [
          "Na máxima extensão permitida por lei, os autores e colaboradores do go-link Console não são responsáveis por quaisquer danos diretos, indiretos, incidentais, especiais, consequentes ou punitivos, nem por perdas de dados, jogos, partidas salvas ou lucros, decorrentes do uso ou da impossibilidade de usar o go-link Console ou seus serviços, nem pelo conteúdo que anfitriões ou convidados carreguem, transmitam ou enviem.",
        ],
        list: [],
      },
      {
        id: "indemnity",
        title: "Indenização",
        body: [
          "Se o seu uso do go-link Console violar estes termos ou a lei, por exemplo ao transmitir arquivos que você não tinha direito de usar, você concorda em isentar os autores e colaboradores do go-link Console de qualquer reclamação decorrente disso.",
        ],
        list: [],
      },
      {
        id: "licenses",
        title: "Licenças",
        body: [],
        list: [
          "O código-fonte próprio do go-link Console é publicado sob a licença MIT.",
          "Os componentes de terceiros incluídos no app e no site do go-link Console mantêm suas próprias licenças, listadas em THIRD_PARTY_NOTICES.md e incluídas em cada versão.",
          "O núcleo do emulador não faz parte do go-link Console: o app baixa o mame2003-plus do buildbot do libretro quando o anfitrião pede. O mame2003-plus está sob a licença não comercial do MAME, então usar o go-link Console com esse núcleo é apenas para uso não comercial: nenhuma distribuição comercial ou monetização do go-link Console junto com esse núcleo é permitida.",
        ],
      },
      {
        id: "trademarks",
        title: "Marcas",
        body: [],
        list: [
          "MAME® é uma marca registrada de Gregory Ember. O go-link Console não é afiliado, endossado nem patrocinado pelo MAMEdev, pela equipe do MAME ou pelo libretro. O nome MAME é usado apenas para descrever com o que o go-link Console é compatível, e o logo do MAME não é usado.",
          "Nomes de empresas, títulos de jogos, logos e personagens que os jogos possam mostrar são marcas e propriedade de seus respectivos donos. O go-link Console é independente e não declara nenhuma afiliação ou endosso de nenhum deles.",
        ],
      },
      {
        id: "reports",
        title: "Denúncias e alterações",
        body: [
          "O go-link Console não hospeda jogos nem conteúdo de usuários, então não pode removê-los. Para denunciar abusos dos servidores do projeto, ou qualquer outra questão sobre estes termos, escreva para lord.basex@gmail.com.",
          "Estes termos podem mudar. A data da versão está no topo desta página, e o site pede que você aceite uma nova versão antes de vincular um dispositivo ou entrar em uma partida novamente.",
        ],
        list: [],
      },
    ],
  },
  privacy: {
    title: "Política de privacidade",
    intro:
      "O go-link Console foi feito para coletar o mínimo possível. Não tem contas, cookies, análises, publicidade nem rastreadores de terceiros, e as fontes do site são servidas pelo próprio site.",
    sections: [
      {
        id: "p2p",
        title: "Jogos, voz e chat viajam ponto a ponto",
        body: [],
        list: [
          "O vídeo, o som do jogo, a voz, os controles e o chat viajam diretamente entre o app do anfitrião e o navegador de cada convidado via WebRTC, que é sempre criptografado (DTLS e SRTP).",
          "Quando não é possível uma conexão direta, esse tráfego passa pelo relay (TURN) do projeto. Ele continua criptografado de ponta a ponta: o relay encaminha pacotes que não consegue ler.",
          "Os servidores do projeto nunca gravam, armazenam ou processam o vídeo, o som, a voz ou o chat de uma partida.",
        ],
      },
      {
        id: "signaling",
        title: "O que o servidor de sinalização vê",
        body: ["Para apresentar o anfitrião e os convidados, o servidor de sinalização processa, apenas em memória e enquanto a conexão durar:"],
        list: [
          "o endereço IP de cada conexão, para conectá-la e aplicar limites contra abusos;",
          "identificadores aleatórios de conexão, códigos de vinculação, identificadores de sala e convites;",
          "a negociação do WebRTC (que inclui endereços de rede) e o PIN da sala digitado por um convidado, que ele repassa sem armazenar;",
          "o identificador do app e um hash do seu segredo, mantidos em memória por até 30 dias para proteger a identidade do app.",
        ],
      },
      {
        id: "logs",
        title: "Registros técnicos",
        body: [
          "Nada do que foi descrito acima é gravado em disco ou mantido depois que o servidor de sinalização reinicia, e ele não registra endereços IP. O relay mantém registros técnicos que podem incluir endereços IP (por exemplo, tentativas de conexão bloqueadas); eles são rotacionados automaticamente, têm tamanho máximo e são usados apenas para operar e proteger o serviço. O site é servido por um provedor de hospedagem que processa os dados usuais de cada requisição (endereço IP, navegador, data) para entregar as páginas.",
        ],
        list: [],
      },
      {
        id: "browser",
        title: "O que o seu navegador guarda",
        body: [
          "O site guarda preferências e chaves no armazenamento local do seu navegador, nunca em cookies nem em nossos servidores: idioma, tema, o servidor de sinalização que você escolheu, o vínculo com o seu app, os passes para voltar às partidas em que entrou, a chave do painel local, ajustes de visualização e som, e a data em que você aceitou estes termos. O editor de skins e o Willy Maker guardam suas skins e seus jogos da mesma forma, no armazenamento local e no banco de dados do próprio navegador (IndexedDB), e eles nunca saem do seu navegador a menos que você os baixe. Ao apagar os dados deste site no navegador, tudo é removido.",
        ],
        list: [],
      },
      {
        id: "host",
        title: "O que o app do anfitrião guarda",
        body: ["O app do anfitrião roda no próprio computador dele, e o anfitrião controla seus dados. Ele guarda, apenas nesse computador:"],
        list: [
          "os navegadores vinculados (apenas um hash de cada chave) e a versão destes termos que cada um aceitou;",
          "o histórico de partidas: cada sala que rodou e, para cada convidado, o nome que digitou, os lugares em que jogou, o caminho da conexão e o endereço IP que o app viu (vazio quando o convidado entrou pelo relay);",
          "o relatório de rede de cada sala: desde que a sala foi ligada pela primeira vez até ser apagada para sempre, uma amostra por segundo de como se comportaram o jogo e a conexão de cada convidado (ida e volta, pacotes perdidos, imagem congelada, o ritmo dos controles, pacotes de voz, o teste de latência) e um registro de eventos (quem entrou e saiu, o nome que usou, o caminho da conexão e o endereço IP, congelamentos); só o anfitrião vê, e apagar a sala para sempre ou uma restauração de fábrica o apaga;",
          "as últimas 50 mensagens do chat de cada sala enquanto ela está aberta, em memória;",
          "gravações, só quando o anfitrião grava uma partida: a imagem e o som do jogo e a voz de cada jogador, em um arquivo no computador do anfitrião. Todos na sala ficam sabendo enquanto é gravado (um aviso REC e uma mensagem no chat). Uma gravação para sozinha após 2 horas ou 2 GB e quando a partida é pausada. Só o anfitrião pode baixá-la ou apagá-la; apagar uma partida do histórico, limpar o histórico ou restaurar de fábrica apaga suas gravações.",
        ],
      },
      {
        id: "addresses",
        title: "Endereços de rede entre jogadores",
        body: [
          "Em uma conexão ponto a ponto, o app do anfitrião e os convidados veem os endereços de rede uns dos outros; é assim que o WebRTC funciona. Se você não quiser compartilhar seu endereço com um anfitrião, não entre nas partidas dele.",
        ],
        list: [],
      },
      {
        id: "rights",
        title: "Seus direitos e servidores próprios",
        body: [
          "Como o go-link Console não tem contas nem guarda dados pessoais em seus servidores além do que está descrito aqui, a maioria dos pedidos se resolve apagando os dados do seu navegador ou pedindo ao anfitrião que apague o histórico. Para qualquer dúvida ou pedido sobre seus dados, escreva para lord.basex@gmail.com.",
          "Qualquer pessoa pode usar o próprio servidor de sinalização. Se você usar um (o site mostra), o operador dele, e não o projeto go-link Console, é o responsável.",
        ],
        list: [],
      },
    ],
  },
  accept: {
    before: "Li e aceito os ",
    terms: "Termos de uso",
    and: " e a ",
    privacy: "Política de privacidade",
    after: ", e só vou usar ROMs que tenho o direito legal de usar.",
    guestAfter: ".",
    required: "Aceite os termos de uso para continuar.",
  },
  footer: {
    tagline: "Jogos arcade transmitidos do seu computador para os navegadores dos seus amigos. Gratuito, de código aberto e sem fins lucrativos.",
    product: "Produto",
    project: "Projeto",
    legal: "Legal",
    source: "Código-fonte",
    releases: "Downloads",
    license: "Licença MIT",
    notices: "Licenças de terceiros",
    legalDoc: "Texto legal completo",
    rights: "go-link Console. Código publicado sob a licença MIT.",
    trademark:
      "MAME® é uma marca registrada de Gregory Ember. O go-link Console não é afiliado nem endossado pelo MAMEdev ou pelo libretro. Os nomes e logos dos jogos pertencem aos seus donos.",
    version: "Versão do site",
    github: "go-link Console no GitHub",
    noRoms: "O go-link Console não hospeda, inclui nem baixa nenhuma ROM, BIOS ou imagem de jogo.",
  },
};
