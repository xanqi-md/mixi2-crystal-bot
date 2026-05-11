require "http/client"
require "json"
require "process"

class MixiBot
  def initialize
    @client_id = ENV["MIXI_CLIENT_ID"]? || ""
    @client_secret = ENV["MIXI_CLIENT_SECRET"]? || ""
    @token_url = ENV["MIXI_TOKEN_URL"]? || "https://application-auth.mixi.social/oauth2/token"
    @api_server = ENV["MIXI_API_SERVER"]? || "application-api.mixi.social"
  end

  def get_access_token
    puts "トークン取得開始..."
    body = "grant_type=client_credentials&client_id=#{@client_id}&client_secret=#{@client_secret}"
    
    begin
      response = HTTP::Client.post(@token_url,
        headers: HTTP::Headers{"Content-Type" => "application/x-www-form-urlencoded"},
        body: body)
      
      if response.status_code == 200
        json = JSON.parse(response.body)
        token = json["access_token"].as_s
        puts "✓ トークン取得成功"
        token
      else
        puts "✗ トークン取得失敗: #{response.status_code}"
        nil
      end
    rescue ex
      puts "エラー: #{ex.message}"
      nil
    end
  end

  def end_of_month?
    tomorrow = Time.local + 1.day
    tomorrow.day == 1
  end

  def run_once
    puts "=== mixi2 ボット実行開始 ==="

    token = get_access_token
    unless token
      puts "✗ トークン取得失敗"
      return
    end

    tomorrow = Time.local + 1.day
    puts "明日の日付: #{tomorrow.day}日"
    puts "月末判定: #{end_of_month?}"

    if end_of_month?
      puts "✓ 今日は月末です！"
      message = "【今日は月末です！ミッションの消化をお忘れなく！】"
    else
      puts "今日は月末ではありません"
      return
    end

    puts "投稿メッセージ: #{message}"
    puts "=== mixi2 ボット実行完了 ==="
  end
end

bot = MixiBot.new
bot.run_once
