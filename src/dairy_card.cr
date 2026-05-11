require "http/client"
require "json"
require "file_utils"

class CardCandidate
  include JSON::Serializable

  property name : String
  property image_url : String
  property source_url : String?

  def initialize(@name : String, @image_url : String, @source_url : String? = nil)
  end
end

class DailyCardPayload
  include JSON::Serializable

  property text : String
  property card_name : String
  property image_url : String
  property source_url : String?
  property date_key : String

  def initialize(@text : String, @card_name : String, @image_url : String, @source_url : String?, @date_key : String)
  end
end

class DailyCardSelector
  IMAGE_BASE = "https://images.ygoprodeck.com/images/cards_cropped"

  def initialize
    @html_url = ENV["DAILY_CARD_HTML_URL"]? || "https://ygogenesys-ja.com/"
    @output_path = ENV["DAILY_CARD_PAYLOAD_PATH"]? || "/tmp/daily_card_payload.json"
    @post_label = ENV["DAILY_CARD_LABEL"]? || "今日の1枚"
  end

  def run
    html = fetch_html(@html_url)
    cards = extract_cards_from_const_cards(html)

    if cards.empty?
      STDERR.puts "const CARDS からカード候補を抽出できませんでした。"
      exit 1
    end

    date_key = jst_date_key
    chosen = select_card(cards, date_key)

    payload = DailyCardPayload.new(
      "#{@post_label}\n#{chosen.name}",
      chosen.name,
      chosen.image_url,
      @html_url,
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

  private def fetch_html(url : String) : String
    response = HTTP::Client.get(url, headers: HTTP::Headers{
      "User-Agent" => "mixi2-crystal-bot/1.0",
    })

    unless response.success?
      raise "HTML 取得失敗: #{response.status_code} #{response.status_message}"
    end

    response.body
  end

  private def extract_cards_from_const_cards(html : String) : Array(CardCandidate)
    marker = "const CARDS = ["
    marker_index = html.index(marker)
    raise "HTML 内に `const CARDS = [` が見つかりません。" unless marker_index

    array_start = marker_index + marker.size - 1
    array_end = find_json_array_end(html, array_start)

    json_text = html.byte_slice(array_start, array_end - array_start + 1)
    parsed = JSON.parse(json_text).as_a

    cards = [] of CardCandidate
    parsed.each do |item|
      obj = item.as_h

      name = as_string(obj["n"]?)
      image_id = as_string(obj["i"]?)

      next if name.nil? || name.not_nil!.strip.empty?
      next if image_id.nil? || image_id.not_nil!.strip.empty?

      cards << CardCandidate.new(
        name.not_nil!.strip,
        "#{IMAGE_BASE}/#{image_id.not_nil!}.jpg",
        @html_url
      )
    end

    cards
  end

  private def as_string(value : JSON::Any?) : String?
    return nil unless value
    raw = value.raw
    case raw
    when String
      raw
    when Int64, Int32, Float64, Float32, Bool
      raw.to_s
    else
      nil
    end
  end

  private def find_json_array_end(text : String, start_index : Int32) : Int32
    bytes = text.to_slice
    i = start_index
    depth = 0
    in_string = false
    escaped = false

    while i < bytes.size
      ch = bytes[i].chr

      if in_string
        if escaped
          escaped = false
        elsif ch == '\\'
          escaped = true
        elsif ch == '"'
          in_string = false
        end
      else
        case ch
        when '"'
          in_string = true
        when '['
          depth += 1
        when ']'
          depth -= 1
          return i if depth == 0
        end
      end

      i += 1
    end

    raise "const CARDS の JSON 配列終端を検出できませんでした。"
  end

  private def jst_date_key : String
    (Time.utc + 9.hours).to_s("%Y-%m-%d")
  end

  private def select_card(cards : Array(CardCandidate), date_key : String) : CardCandidate
    rng = Random.new(date_key.hash)
    cards[rng.rand(cards.size)]
  end
end

DailyCardSelector.new.run
