#!/system/bin/sh
SKIPUNZIP=1
unzip -o "$ZIPFILE" -d "$MODPATH" >&2 || abort '! Cannot extract Magicap'
MODDIR=$MODPATH
export MODDIR
. "$MODDIR/lib/kamfw/.kamfwrc" || abort '! Cannot load kamfw'
kamfw_phase_install() {
    set_perm_recursive "$MODPATH" 0 0 0755 0644
    set_perm_recursive "$MODPATH/bin" 0 0 0755 0755
    for script in service.sh uninstall.sh action.sh; do
        set_perm "$MODPATH/$script" 0 0 0755
    done
    # Recreate explicitly: ZIP installers differ in symlink handling.
    rm -f "$MODPATH/cli"
    ln -s bin/magicap-cli "$MODPATH/cli" || abort '! Cannot create cli symlink'
    "$MODPATH/cli" cert overlay || abort '! Cannot initialize per-device CA'
    set_perm_recursive "$MODPATH/system" 0 0 0755 0644
    set_i18n MAGICAP_INSTALLED zh 'Magicap 已安装。抓包和 MCP 默认关闭；配置后重启加载证书。' en 'Magicap installed. Capture and MCP default off. Reboot to load the CA.'
    success "$(i18n MAGICAP_INSTALLED)"
}
kamfw run install -- "$@"
