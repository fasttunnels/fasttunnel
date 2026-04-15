package commands

import (
	"fmt"
)

const bashCompletion = `# bash completion for fasttunnel
_fasttunnel()
{
    local cur prev words cword
    _init_completion || return

    local commands="http https login completion version"

    case "${prev}" in
        completion)
            COMPREPLY=( $(compgen -W "zsh bash fish" -- "${cur}") )
            return
            ;;
        -p|--port)
            COMPREPLY=()
            return
            ;;
        -s|--subdomain)
            COMPREPLY=()
            return
            ;;
        -c|--callback-port)
            COMPREPLY=()
            return
            ;;
    esac

    if [[ ${cword} -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
        return
    fi

    local cmd="${words[1]}"
    case "${cmd}" in
        http|https)
            COMPREPLY=( $(compgen -W "-p --port -s --subdomain" -- "${cur}") )
            ;;
        login)
            COMPREPLY=( $(compgen -W "-c --callback-port" -- "${cur}") )
            ;;
        completion)
            COMPREPLY=( $(compgen -W "zsh bash fish" -- "${cur}") )
            ;;
        *)
            COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
            ;;
    esac
}
complete -F _fasttunnel fasttunnel
`

const zshCompletion = `#compdef fasttunnel

_fasttunnel() {
  local -a commands
  commands=(
    'http:Expose local HTTP app'
    'https:Expose local HTTPS app'
    'login:Authenticate CLI'
    'completion:Print shell completion script'
    'version:Show version info'
  )

  if (( CURRENT == 2 )); then
    _describe 'command' commands
    return
  fi

  case "$words[2]" in
    http|https)
      _arguments \
        '1:port:' \
        '(-p --port)'{-p,--port}'[local port]:port:' \
        '(-s --subdomain)'{-s,--subdomain}'[subdomain]:subdomain:'
      ;;
    login)
      _arguments \
        '(-c --callback-port)'{-c,--callback-port}'[callback port]:port:'
      ;;
    completion)
      _values 'shell' zsh bash fish
      ;;
    version)
      ;;
    *)
      _describe 'command' commands
      ;;
  esac
}

_fasttunnel "$@"
`

const fishCompletion = `# fish completion for fasttunnel
complete -c fasttunnel -f

complete -c fasttunnel -n "__fish_use_subcommand" -a "http" -d "Expose local HTTP app"
complete -c fasttunnel -n "__fish_use_subcommand" -a "https" -d "Expose local HTTPS app"
complete -c fasttunnel -n "__fish_use_subcommand" -a "login" -d "Authenticate CLI"
complete -c fasttunnel -n "__fish_use_subcommand" -a "completion" -d "Print shell completion script"
complete -c fasttunnel -n "__fish_use_subcommand" -a "version" -d "Show version info"

complete -c fasttunnel -n "__fish_seen_subcommand_from http https" -s p -l port -d "Local port"
complete -c fasttunnel -n "__fish_seen_subcommand_from http https" -s s -l subdomain -d "Subdomain"
complete -c fasttunnel -n "__fish_seen_subcommand_from login" -s c -l callback-port -d "Callback port"

complete -c fasttunnel -n "__fish_seen_subcommand_from completion" -a "zsh bash fish" -d "Target shell"
`

// RunCompletion prints a shell completion script for the requested shell.
func RunCompletion(shell string) error {
	switch shell {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		return fmt.Errorf("unsupported shell %q", shell)
	}
	return nil
}
