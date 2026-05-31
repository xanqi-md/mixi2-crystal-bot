require "http/client"
require "json"
require "time"

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

  def is_noon_in_jst?
    # UTCの現在時刻を取得
    utc_now = Time.utc_now
    
    # JSTに変換（UTC+9）
    jst_now = utc_now.in(Time::Location.fixed(9 * 3600))
    
    # JSTの時間が12時かどうかを判定
    jst_now.hour == 12
  end

  def end_of_month?
    tomorrow = Time.utc_now.in(Time::Location.fixed(9 * 3600)) + 1.day
    tomorrow.day == 1
  end

  def run_once
    puts "=== mixi2 Plugin ボット実行開始 ==="

    token = get_access_token
    unless token
      puts "✗ トークン取得失敗"
      return
    end

    jst_now = Time.utc_now.in(Time::Location.fixed(9 * 3600))
    tomorrow = jst_now + 1.day
    
    puts "現在時刻（JST）: #{jst_now}"
    puts "明日の日付: #{tomorrow.day}日"
    puts "月末判定: #{end_of_month?}"
    puts "12時判定: #{is_noon_in_jst?}"

    if end_of_month? && is_noon_in_jst?
      puts "✓ 今日は月末の12時です！投稿します"
      # ここに投稿処理を追加
    else
      puts "投稿条件を満たしていません"
      return
    end

    puts "=== mixi2 Plugin ボット実行完了 ==="
  end
end

bot = MixiBot.new
bot.run_once
