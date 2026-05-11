require "http/client"
require "json"
require "file_utils"
require "set"

class CardCandidate
  include JSON::Serializable

  getter name : String
  getter image_url : String
  getter source_url : String?

  def initialize(@name : String, @image_url : String, @source_url : String? = nil)
  end
end

class DailyCardPayload
  include JSON::Serializable

  getter text : String
  getter card_name : String
  getter image_url : String
  getter source_url : String?
  getter date_key : String

  def initialize(
    @text : String,
    @card_name : String,
    @image_url : String,
    @source_url : String?,
    @date_key : String
  )
  end
end

class DailyCardSelector
  DEFAULT_HTML_URL       = "https://ygogenesys-ja.com/"
  DEFAULT_IMAGE_BASE_URL = "https://images.ygoprodeck.com/images/cards_cropped"

  def initialize
    @html_url = ENV["DAILY_CARD_HTML_URL"]?
    @html_file = ENV["DAILY_CARD_HTML_FILE"]?
    @image_base_url = ENV["DAILY_CARD_IMAGE_BASE_URL"]? || DEFAULT_IMAGE_BASE_URL
    @output_path = ENV["DAILY_CARD_PAYLOAD_PATH"]? || "/tmp/daily_card_payload.json"
    @post_label = ENV["DAILY_CARD_LABEL"]? || "今日の1枚"
  end

  def run
    html, source_url = load_html_and_source
    cards = extract_cards_from_const_cards(html, source_url)

    if cards.empty?
      STDERR.puts "カード候補を抽出できませんでした。const CARDS の構造を確認してください。"
      exit 1
    end

    date_key = jst_date_key
    chosen = select_card(cards, date_key)

    payload = DailyCardPayload.new(
      "#{@post_label}\n#{chosen.name}",
      chosen.name,
      chosen.image_url,
      chosen.source_url,
      date_key
    )

    FileUtils.mkdir_p(File.dirname(@output_path))
    File.write(@output_path, payload.to_json)

    puts "候補数: #{cards.size}"
    puts "選択日: #{date_key}"
    puts "カード名: #{chosen.name}"
    puts "画像URL: #{chosen.image_url}"
    puts "payload: #{@output_path}"
  end

  private def load_html_and_source : Tuple(String, String?)
    if html_file = @html_file
      return {File.read(html_file), nil}
    end

    target_url = @html_url || DEFAULT_HTML_URL
    response = HTTP::Client.get(
      target_url,
      headers: HTTP::Headers{
        "User-Agent" => "mixi2-crystal-bot/1.0",
      }
    )

    unless response.success?
      raise "HTML 取得失敗: #{response.status_code} #{response.status_message}"
    end

    {response.body, target_url}
  end

  private def extract_cards_from_const_cards(html : String, source_url : String?) : Array(CardCandidate)
    json_array_text = extract_cards_json_array(html)

    puts "CARDS JSON 先頭: #{json_array_text[0, Math.min(120, json_array_text.size)]}"
    puts "CARDS JSON 末尾: #{json_array_text[Math.max(0, json_array_text.size - 120), Math.min(120, json_array_text.size)]}"

    raw_cards = JSON.parse(json_array_text).as_a

    cards = [] of CardCandidate
    seen = Set(String).new

    raw_cards.each do |entry|
      obj = entry.as_h

      name = obj["n"]?.try(&.as_s?) || ""
      image_id = obj["i"]?.try(&.as_s?) || ""

      name = clean_text(name)
      image_id = image_id.strip

      next if name.empty?
      next if image_id.empty?

      image_url = build_image_url(image_id)
      key = "#{name}\u0000#{image_url}"
      next if seen.includes?(key)

      seen.add(key)
      cards << CardCandidate.new(name, image_url, source_url)
    end

    cards
  end

  private def extract_cards_json_array(html : String) : String
    start_marker = "const CARDS = "
    end_marker = "const META ="

    start_index = html.index(start_marker)
    raise "#{start_marker} が見つかりませんでした" unless start_index

    body_start = start_index + start_marker.size

    end_index = html.index(end_marker, body_start)
    raise "#{end_marker} が見つかりませんでした" unless end_index

    json_text = html[body_start...end_index].strip

    if json_text.ends_with?(";")
      json_text = json_text[0, json_text.size - 1].rstrip
    end

    unless json_text.starts_with?("[")
      raise "CARDS JSON の先頭が '[' ではありません"
    end

    unless json_text.ends_with?("]")
      raise "CARDS JSON の末尾が ']' ではありません"
    end

    json_text
  end

  private def build_image_url(image_id : String) : String
    "#{@image_base_url}/#{image_id}.jpg"
  end

  private def clean_text(text : String) : String
    text
      .gsub(/<[^>]+>/, "")
      .gsub(/\s+/, " ")
      .strip
  end

  private def jst_date_key : String
    (Time.utc + 9.hours).to_s("%Y-%m-%d")
  end

  private def select_card(cards : Array(CardCandidate), date_key : String) : CardCandidate
    index = stable_index(date_key, cards.size)
    cards[index]
  end

  private def stable_index(text : String, size : Int32) : Int32
    hash = 2166136261_u32

    text.each_byte do |byte|
      hash ^= byte.to_u32
      hash &*= 16777619_u32
    end

    (hash % size.to_u32).to_i
  end
end

DailyCardSelector.new.run
