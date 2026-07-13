package main

// Shell completion scripts. They complete subcommands and tool names statically,
// and profile names dynamically by shelling out to the hidden `charon __profiles`.
// Kept free of backticks so they can live in Go raw string literals.

const bashCompletion = `# bash completion for charon
_charon() {
    local cur cword cmds tools sub
    cur="${COMP_WORDS[COMP_CWORD]}"
    cword=$COMP_CWORD
    cmds="status ls save models add edit rename cp switch use restore undo prune rm completion version help"
    tools="codex claude opencode pi"

    if [ "$cword" -eq 1 ]; then
        COMPREPLY=( $(compgen -W "$cmds" -- "$cur") )
        return
    fi

    sub="${COMP_WORDS[1]}"
    case "$sub" in
        switch|use|rm|edit|rename|cp)
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=( $(compgen -W "$tools" -- "$cur") )
            elif [ "$cword" -eq 3 ]; then
                COMPREPLY=( $(compgen -W "$(charon __profiles "${COMP_WORDS[2]}" 2>/dev/null)" -- "$cur") )
            fi
            ;;
        ls|save|models|add|status|undo|prune|restore)
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=( $(compgen -W "$tools" -- "$cur") )
            fi
            ;;
        completion)
            if [ "$cword" -eq 2 ]; then
                COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
            fi
            ;;
    esac
}
complete -F _charon charon
`

const zshCompletion = `#compdef charon
# zsh completion for charon
_charon() {
    local -a cmds tools
    cmds=(status ls save models add edit rename cp switch use restore undo prune rm completion version help)
    tools=(codex claude opencode pi)

    if (( CURRENT == 2 )); then
        compadd -- $cmds
        return
    fi

    case ${words[2]} in
        switch|use|rm|edit|rename|cp)
            if (( CURRENT == 3 )); then
                compadd -- $tools
            elif (( CURRENT == 4 )); then
                compadd -- ${(f)"$(charon __profiles ${words[3]} 2>/dev/null)"}
            fi
            ;;
        ls|save|models|add|status|undo|prune|restore)
            (( CURRENT == 3 )) && compadd -- $tools
            ;;
        completion)
            (( CURRENT == 3 )) && compadd bash zsh fish
            ;;
    esac
}
compdef _charon charon
`

const fishCompletion = `# fish completion for charon
function __charon_needs_tool
    set -l cmd (commandline -opc)
    if test (count $cmd) -eq 2
        switch $cmd[2]
            case switch use rm edit rename cp ls save models add status undo prune restore
                return 0
        end
    end
    return 1
end

function __charon_needs_profile
    set -l cmd (commandline -opc)
    if test (count $cmd) -eq 3
        switch $cmd[2]
            case switch use rm edit rename cp
                return 0
        end
    end
    return 1
end

function __charon_profiles
    set -l cmd (commandline -opc)
    if test (count $cmd) -ge 3
        charon __profiles $cmd[3] 2>/dev/null
    end
end

complete -c charon -f
complete -c charon -n '__fish_use_subcommand' -a 'status ls save models add edit rename cp switch use restore undo prune rm completion version help'
complete -c charon -n '__charon_needs_tool' -a 'codex claude opencode pi'
complete -c charon -n '__charon_needs_profile' -a '(__charon_profiles)'
`
