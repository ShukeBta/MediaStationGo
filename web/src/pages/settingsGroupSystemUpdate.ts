import type { SettingGroup } from './settingsGroupTypes'

export const systemUpdateSettingsGroup: SettingGroup = {
  key: 'system-update',
  label: '系统更新',
  description: 'Docker Compose 部署可在这里检查并拉取最新版镜像。',
  items: [
    {
      key: 'system.update.image',
      label: '应用镜像',
      type: 'text',
      defaultValue: 'ghcr.io/shukebta/mediastation-go:latest',
      placeholder: 'ghcr.io/shukebta/mediastation-go:latest',
      hint: '用于检查远端摘要；保持 latest 即可跟随主分支镜像。',
    },
    {
      key: 'system.update.compose_dir',
      label: 'Docker Compose 安装目录',
      type: 'text',
      placeholder: '/vol1/1000/docker/mediastation-go',
      hint: '留空时自动查找 docker-compose.yml / compose.yml 所在目录。',
    },
    {
      key: 'system.update.command',
      label: '自定义更新命令',
      type: 'textarea',
      placeholder:
        'cd {{compose_dir}} && {{compose_command}} pull && {{compose_command}} up -d && docker image prune -f && docker restart {{container}}',
      hint: '留空时使用 Docker Compose 默认命令。支持 {{image}}、{{compose_dir}}、{{compose_file}}、{{compose_command}}、{{container}}、{{container_id}}、{{container_name}}。',
    },
  ],
}
